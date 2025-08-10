# Repository Guidelines

This project maintains a set of conventions to keep contributions consistent and maintainable.

## Logging

- Use [zap](https://github.com/uber-go/zap) for structured logging.
- Zap is the sole logging backend; avoid `log` or `fmt.Print*` for progress output.
- Always call `Sync()` (e.g., `defer logger.Sync()`) before program exit to flush buffers.
- Transport constructors must accept a `*zap.Logger`; avoid package-level loggers.
- Log connection lifecycle events and errors with `snake_case` fields including units (e.g., `bytes_transferred`, `duration_ms`).
- Callers using transports should `defer logger.Sync()` to ensure logs are flushed.

### Field Naming

- Use `snake_case` for all field keys.
- Include the unit in the name when relevant:
  - `resource_id` for identifiers
  - `duration_ms` for durations in milliseconds
  - `size_bytes` for sizes

### Log Levels

- `Debug` for verbose details useful during development.
- `Info` for lifecycle events and high-level progress.
- `Warn` for unexpected situations that do not stop execution.
- `Error` for failures that require user action.
- Avoid `Fatal` in libraries; return an error instead.

### Example

```go
logger.Info("snapshot complete",
    zap.String("resource_id", snapshotID),
    zap.Int64("duration_ms", time.Since(start).Milliseconds()),
)
```

## CLI Patterns

- Prefer [`pflag`](https://github.com/spf13/pflag) for flag parsing.
- Bind flags to [`viper`](https://github.com/spf13/viper) to support configuration from flags, config files, and environment variables.
- Group related options into `FlagSet`s to share common configuration across commands.
- Define flags within `pflag.FlagSet`s and bind them to `viper`; the standard library `flag` package is not used.
- Expose configuration via both config files and environment variables for easy automation.

### Flag Grouping Example

```go
func initConfig() *viper.Viper {
    v := viper.New()
    syncFlags := pflag.NewFlagSet("sync", pflag.ExitOnError)
    syncFlags.String("source", "", "snapshot device")
    syncFlags.String("dest", "", "destination path")
    pflag.CommandLine.AddFlagSet(syncFlags)

    v.BindPFlags(syncFlags)
    v.SetConfigName("config")
    v.AddConfigPath(".")
    v.SetEnvPrefix("LVMSYNC")
    v.AutomaticEnv()
    return v
}
```

Sample `config.yaml`:

```yaml
source: /dev/vg0/snap0
dest: /mnt/backup
```

Environment override:

```sh
LVMSYNC_SOURCE=/dev/vg0/snap1
```

## Transport Configuration

Example usage selecting transports and ports:

```sh
lvmsync --transport quic,h2,tcp+tls,ssh --quic-connect host:9000 --tcp-port 9443
```

BDP-based autotuning keeps roughly one to two times the bandwidth–delay product
in flight. Override the autotuned value with `--concurrency`.

## Throughput Mode

The `throughput` preset favors maximal transfer rates:

- transport order `quic,h2,tcp+tls`
- concurrency `8`
- hybrid dedup with 2 MiB fixed chunks and CDC range 256 KiB–8 MiB
- compression `auto`
- enables `--odirect`
- `--sync-interval=1 GiB`, `--checkpoint-bytes=1 GiB`, `--checkpoint-interval=10s`
- QUIC congestion control `bbr`

## Compression Policy

- Sample 8 KiB from each chunk to estimate the compression ratio.
- Skip compression when the ratio is greater than or equal to `--compress_threshold`.
- Auto mode selects LZ4 for chunks under 256 KiB and Zstd level 1 for larger chunks on AVX2-capable CPUs.
- Choose the algorithm with `--compress {auto|lz4|zstd|none}` and tune levels using `--zstd_level 1..5` or `--lz4_level {fast|hc}`.

Example configuration:

```sh
lvmsync --compress auto --zstd_level 2 --compress_threshold 0.85
```

## Performance Libraries

- Prefer CPU-accelerated implementations such as `github.com/zeebo/blake3` for hashing and `github.com/klauspost/compress` for Zstd and LZ4 compression.
- Choose libraries that leverage vector instructions when available to maximize throughput.

## Coding Conventions

### Transports

- Register transports via `transport.Register` using short, descriptive names.
- Each transport must implement `Sender` and `Receiver` interfaces and include an integration test.
- Avoid global state; pass configuration and loggers explicitly.

### Deduplication

- Dedup strategies should expose pure functions and persist state deterministically.
- Provide tests for Bloom filter sizing and CDC window boundaries.
- Document tunables `--cdc-min`, `--cdc-avg`, `--cdc-max`, and `--bloom-mbits`.
- Verify configuration precedence (flags > env vars > config file) and handle invalid YAML or value parse errors.

### Compression

- Detect CPU features before selecting algorithms.
- Sample 8 KiB per chunk to estimate ratios and honour `--compress_threshold`.
- Benchmarks must cover algorithm choice and cache resets.

## Control Plane Flow

- Clients perform a handshake sending `sector_size`, `alignment`, `max_concurrency`, and dedup/compression support.
- Sessions exchange ephemeral X.509 certificates and a pre-shared key.
- Resume bitmaps are streamed with a session ID to continue interrupted transfers.
- Each session maintains a bidirectional `Ack` stream for pings and acknowledgements.
- Finalization requests close the session by ID.

## Resume Workflow

- Resume state files record the BLAKE3 digest of the last CDC chunk.
- Checkpoint the resume file every 1 GiB of raw data or every 10 s.
- Exchange final manifests containing chunk hashes and a final SHA-256 digest to verify completion.

## Testing and Linting

- Ensure [`golangci-lint`](https://golangci-lint.run/) v2 passes before submitting changes.

### Coverage

- Run `go test -cover ./...` to generate coverage statistics.
- For a file `coverage.out`, view details with `go tool cover -func=coverage.out`.
- Ensure overall test coverage remains at or above 50%; CI enforces this threshold and fails if the total coverage falls below it.

## Unit Tests

- Every function must have a dedicated unit test.
- Cover both successful and failing paths to verify correctness.
- Where external commands would normally execute, inject test hooks (e.g.,
  `privesc.EnsureRoot` accepts an `exec` function) to stub side effects during
  tests.

## Modularity and Single Responsibility

- Keep packages and functions focused on a single task.
- Break up large files or components when functionality grows.

## Configuration Documentation

- Document every CLI flag, environment variable, and configuration option in `README.md`.
- Update examples and default values to match new options.
- Deduplication modes (`fixed`, `cdc`, `hybrid`) expose tunables `--cdc-min`, `--cdc-avg`, `--cdc-max`, and Bloom filter sizing with `--bloom-mbits` for the mmap-backed index.
- Keep transport sections and flag-to-env tables (QUIC, HTTP/2, TCP+TLS, SSH) in sync with code changes.

## Release Workflow

- Tags follow `vX.Y.Z` semantic versioning.
- Pushing a tag triggers `.github/workflows/release.yml` which builds and publishes artifacts.
- Each release must include a `CHANGELOG.md` entry using the [Keep a Changelog](https://keepachangelog.com/) format:

  ```markdown
  ## [vX.Y.Z] - YYYY-MM-DD
  ### Added
  - ...
  ### Fixed
  - ...
  ```

## Contribution and Commit Messages

- Follow the guidelines in `CONTRIBUTING.md`.
- Commit messages must follow the Conventional Commits format: `type(scope): description`.

## TODO

- [ ] Audit logging: ensure `zap` is used with `snake_case` fields, include units, and call `logger.Sync()` before exit (block size logs now use `size_bytes`).
- [ ] Remove package-wide loggers; transport constructors such as `ssh.New(cfg, logger)` now require an explicit `*zap.Logger` parameter.
- [ ] Remove stray `fmt.Print*` calls in favor of structured logs.
- [ ] Monitor elimination of `fmt.Print*` calls to keep progress logging fully structured.
- [ ] Review QUIC constructor refactor and expand tests for sender/receiver coverage.
- [x] Refactor `cmd/grpcd` to defer `syncLogger` for structured log flushing.
- [x] Expand unit test coverage for remote execution and client signal handling, and run coverage reports (transports coverage ≥50%).

  ```sh
  go test -cover ./...
  ```

- [x] Run lint checks to keep style and correctness issues from creeping in for transports.

  ```sh
  golangci-lint run
  ```

 - [ ] Review CLI argument parsing: prefer `pflag`, bind flags to `viper`, and group related options into reusable `FlagSet`s. Further flag-binding audits remain for commands beyond `config` and `cmd/grpcd`.
- [ ] Implement real transports for QUIC, HTTP/2, TCP+TLS, and SSH; replace placeholders with functional backends and tests.
- [ ] Add privilege escalation (`privesc`) tests covering success and error paths.
- [ ] Expand coverage for configuration precedence across flags, environment variables, and config files.
 - [ ] Keep modules single-purpose; maintain the `transfer` package decomposition (`progress.go`, `handshake.go`, `block_writer.go`).
- [ ] Document gRPC daemon configuration sources (flags, env vars, config file) and precedence.
- [ ] Keep `README` configuration documentation current with code changes.
- [ ] Keep transport documentation and configuration references (flags and env vars) up to date.
- [ ] Track decomposition of large files like `transfer/transfer.go`.
- [ ] Ensure progress logging uses `zap` exclusively.
 - [ ] Ensure every function has a dedicated unit test.
- [ ] Maintain tests for compression detection, ensuring benchmark and cache logic remain correct.
- [ ] Maintain tests for buffer alignment, hole punching, and NUMA pinning.
- [ ] Enforce modular, single-responsibility design across packages.
- [ ] Document each new CLI flag, environment variable, and configuration option in `README.md`.
- [x] Refactor `main.go` into smaller modules.
- `cmd/dump` handles snapshot dumping and transport selection, receiving configuration and loggers explicitly.
- `cmd/root` configures the application and wires `cmd/dump` and `cmd/apply`.
- `cmd/apply` streams incoming data to destination devices and also accepts explicit configuration and loggers.
- [ ] Verify configuration precedence.
- [ ] Maintain tests for dedup configuration precedence and error paths (invalid YAML, parse errors).
- [ ] Keep README dedup configuration options in sync with code.
- [ ] Document feature changes in `README.md`.
- [x] Implement full transport registry with working QUIC, HTTP/2, TCP+TLS, and SSH backends and accompanying tests.
- [x] Finalize hybrid deduplication and document CDC tuning knobs.
- [x] Integrate adaptive compression sampling with configurable thresholds and unit benchmarks.
## Roadmap

- [ ] Implement gRPC control plane with mTLS and port configurability.
- [x] Introduce pluggable data plane with transport registry supporting QUIC, HTTP/2, TLS/TCP, and SSH.
- [x] Add hybrid deduplication combining fixed-size and content-defined chunking (FastCDC).
- [x] Implement adaptive compression with CPU feature detection and per-chunk sampling.
- [ ] Optimize transfer pipeline for concurrency autotuning, large in-flight windows, and efficient I/O paths.
- [ ] Provide `FlagSet`-grouped CLI options bound to Viper, with parity across flags, environment variables, and config files.
- [ ] Ensure each new function includes unit tests and documentation updates in README.md.
- [ ] Maintain modular, single-responsibility design to ease future maintenance.
