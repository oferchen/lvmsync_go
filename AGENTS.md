# Repository Guidelines

This project maintains a set of conventions to keep contributions consistent and maintainable.

## Logging

- Use [zap](https://github.com/uber-go/zap) for structured logging.
- Always call `Sync()` (e.g., `defer logger.Sync()`) before program exit to flush buffers.

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

## Compression Policy

- Sample 8 KiB from each chunk to estimate the compression ratio.
- Skip compression when the ratio is greater than or equal to `--compress_threshold`.
- Auto mode selects LZ4 for chunks under 256 KiB and Zstd level 1 for larger chunks on AVX2-capable CPUs.
- Choose the algorithm with `--compress {auto|lz4|zstd|none}` and tune levels using `--zstd_level 1..5` or `--lz4_level {fast|hc}`.

Example configuration:

```sh
lvmsync --compress auto --zstd_level 2 --compress_threshold 0.85
```

## Control Plane Flow

- Clients perform a handshake sending `sector_size`, `alignment`, `max_concurrency`, and dedup/compression support.
- Sessions exchange ephemeral X.509 certificates and a pre-shared key.
- Resume bitmaps are streamed with a session ID to continue interrupted transfers.
- Each session maintains a bidirectional `Ack` stream for pings and acknowledgements.
- Finalization requests close the session by ID.

## Testing and Linting

- Ensure [`golangci-lint`](https://golangci-lint.run/) v2 passes before submitting changes.

### Coverage

- Run `go test -cover ./...` to generate coverage statistics.
- For a file `coverage.out`, view details with `go tool cover -func=coverage.out`.
- Enforce a minimum threshold by failing the build when the total coverage is below the desired percentage.

## Unit Tests

- Add a dedicated unit test for every new function.
- Cover both successful and failing paths to verify correctness.

## Modularity and Single Responsibility

- Keep packages and functions focused on a single task.
- Break up large files or components when functionality grows.

## Configuration Documentation

- Document every CLI flag, environment variable, and configuration option in `README.md`.
- Update examples and default values to match new options.
- Deduplication modes (`fixed`, `cdc`, `hybrid`) expose tunables `--cdc-min`, `--cdc-avg`, `--cdc-max`, and Bloom filter sizing with `--bloom-mbits` for the mmap-backed index.

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

- [ ] Audit logging: ensure `zap` is used with `snake_case` fields, include units, and call `logger.Sync()` before exit.
- [ ] Remove stray `fmt.Print*` calls in favor of structured logs.
- [ ] Monitor elimination of `fmt.Print*` calls to keep progress logging fully structured.
- [ ] Expand unit test coverage for remote execution and client signal handling, and run coverage reports.

  ```sh
  go test -cover ./...
  ```

- [ ] Run lint checks to keep style and correctness issues from creeping in.

  ```sh
  golangci-lint run
  ```

- [ ] Review CLI argument parsing: prefer `pflag`, bind flags to `viper`, and group related options into reusable `FlagSet`s.
- [ ] Keep modules single-purpose; maintain the `transfer` package decomposition (`progress.go`, `handshake.go`, `block_writer.go`).
- [ ] Document gRPC daemon configuration sources (flags, env vars, config file) and precedence.
- [ ] Keep `README` configuration documentation current with code changes.
- [ ] Track decomposition of large files like `transfer/transfer.go`.
- [ ] Ensure progress logging uses `zap` exclusively.
- [ ] Add a dedicated unit test for every new function.
- [ ] Enforce modular, single-responsibility design across packages.
- [ ] Document each new CLI flag, environment variable, and configuration option in `README.md`.
- [ ] Refactor `main.go` into smaller modules.
- [ ] Verify configuration precedence.
- [ ] Document feature changes in `README.md`.
