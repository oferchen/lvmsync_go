# Repository Guidelines

This project maintains a set of conventions to keep contributions consistent and maintainable.

## Context

LVMSync is a high-performance tool for streaming LVM snapshots across hosts. The codebase targets
production reliability, favoring small, single-purpose components with clear interfaces. Each
function should do one thing well, include dedicated tests, and rely on dependency injection where
practical. Break down large functions into focused helpers and test each helper individually.
Logging and configuration are fully structured to keep behavior predictable across
command-line use, environment variables, and `config.yaml` files.

## Dependency Injection

- Avoid package-level function variables for stubbing dependencies.
- Use a `Runner` struct to hold external interactions and provide `NewRunner` and
  `NewRunnerWithDeps` constructors so tests can inject mocks.
- Callers and subcommands should invoke methods on a `Runner` instance rather than
  modifying global variables.

## Device Detection

- `Detect` orchestrates `detectFileDevice`, `detectLVMDevice`, and `detectRawDevice`.
- Each helper includes dedicated tests for success and error paths.
- Non-root runs escalate LVM operations using `--lvm-escalation` (default `sudo -n`). The command is validated at startup and detection fails if escalation is unavailable.

## Logging

- Use [zap](https://github.com/uber-go/zap) for structured logging.
- Zap is the sole logging backend; avoid `log` or `fmt.Print*` for progress output.
- Always flush logs using the `SyncLogger` helper (e.g., `defer rootcmd.SyncLogger(logger)`) so `logger.Sync()` errors are recorded.
- Transport constructors must accept a `*zap.Logger`; avoid package-level loggers.
- Pass loggers explicitly to commands and helpers; do not use `zap.L()` or other globals.
- Use `zap.NewNop()` instead of `nil` when no logging is needed.
- Log connection lifecycle events and errors with `snake_case` fields including units (e.g., `bytes_transferred`, `duration_ms`).
- Do not log secrets or authentication tokens; scrub sensitive values before emitting them.
- Callers using transports should `defer rootcmd.SyncLogger(logger)` to ensure logs are flushed.

### Logging utilities

- `TLSVersionString` returns `"unknown"` or the numeric TLS version when the value is unrecognized to avoid empty strings in logs.

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
- New flag groups must include tests demonstrating configuration precedence.
- `--fs-freeze-command` and `--fs-thaw-command` values are split with shell-style quoting; tests should cover paths with spaces and README examples must quote such arguments.

### Flag Grouping Example

```go
func initConfig() (*viper.Viper, string, string) {
    v := viper.New()
    general := pflag.NewFlagSet("general", pflag.ExitOnError)
    general.Bool("progress", true, "show progress")
    pflag.CommandLine.AddFlagSet(general)

    v.BindPFlags(pflag.CommandLine)
    v.SetConfigName("config")
    v.AddConfigPath(".")
    v.SetEnvPrefix("LVMSYNC")
    v.AutomaticEnv()

    pflag.Parse()
    args := pflag.Args()
    if len(args) < 2 {
        pflag.Usage()
        os.Exit(1)
    }
    return v, args[0], args[1]
}
```

Example invocation:

```sh
lvmsync /dev/vg0/snap0 /mnt/backup
```

## Transport Configuration

Example usage selecting transports and ports:

```sh
lvmsync --transport quic,h2,tcp+tls,ssh --tcp-port 9443
```

BDP-based autotuning keeps roughly one to two times the bandwidth–delay product
in flight. Override the autotuned value with `--concurrency`.


- TLS 1.3 with mutual authentication is required by default; `AllowInsecure`
  is for development only.
  `--allow-insecure` (defaults to false).

## Throughput Mode

The `throughput` preset favors maximal transfer rates:

- transport order `quic,h2,tcp+tls,ssh`
- concurrency `8`
- hybrid dedup with 2 MiB fixed chunks and CDC range 256 KiB–8 MiB
- compression `auto`
- enables `--odirect`
- `--sync-interval=1 GiB`, `--checkpoint-bytes=1 GiB`, `--checkpoint-interval=10s`
- QUIC congestion control `bbr`

## Compression Policy

- Sample 8 KiB from each chunk to estimate the compression ratio.
- Skip compression when the ratio is greater than or equal to `--compress-threshold`.
- Auto mode selects LZ4 for chunks under 256 KiB and Zstd for larger chunks on CPUs with AVX2 or NEON support.
- Choose the algorithm with `--compress {auto|lz4|zstd|none}` and tune levels using `--zstd-level 1..5` or `--lz4-level {fast|hc}`.

Example configuration:

```sh
lvmsync --compress auto --zstd-level 2 --compress-threshold 0.85
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
- Sample 8 KiB per chunk to estimate ratios and honour `--compress-threshold`.
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
    the `privilege` package accepts `exec.Command` substitutes) to stub side
    effects during tests.
- The snapshot monitoring goroutine closes its error channel on exit; cleanup
  must only cancel monitoring. `TestCreateSnapshotCleanupNoPanic` verifies this
  behavior.

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
- Every pull request must update the `[Unreleased]` section in `CHANGELOG.md` with entries under `Added` or `Fixed`.

## Contribution and Commit Messages

- Follow the guidelines in `CONTRIBUTING.md`.
- Commit messages must follow the Conventional Commits format: `type(scope): description`.

## Pre-Merge Checklist

Run these commands locally before opening a pull request:

- `go build ./...`
- `go test -coverprofile=coverage.out ./...`
- `golangci-lint run`
- `go tool cover -func=coverage.out` and ensure total coverage is at least 50%
- Include unit tests covering success and failure paths for new functionality.

## Production Readiness Checklist

- Use `zap` for all structured logging and call `rootcmd.SyncLogger(logger)` on shutdown.
- Parse configuration with `pflag` and `viper`; expose every option via CLI flags, `LVMSYNC_*` environment variables, and the
  `config.yaml` file.
- Group related CLI options into dedicated `FlagSet`s with clear descriptions.
- Keep packages and functions single-purpose and inject dependencies for testability.
- Provide unit tests for every function, covering success and error paths.
- Run `go build ./...`, `go test -cover ./...`, and `golangci-lint run` before merging.
- Document new flags, environment variables, and configuration options in `README.md`.

## Gap Reporting

- Track known issues in `reports/gaps.md` and `reports/gaps.json`.
- Each gap entry must appear in both files with matching details.
- `reports/gaps_test.go` fails `go test` when `gaps.json` is non-empty.
- Remove entries after fixes to keep CI green.

## Regex Patterns

- Remote command validation uses a precompiled regex `^[a-zA-Z0-9._-]+$` (`remoteCmdRe` in `remote/remote.go`). Maintain this pattern when checking remote commands.

## TODO

- [ ] Audit logging: ensure `zap` is used with `snake_case` fields, include units, and call `rootcmd.SyncLogger(logger)` before exit (block size logs now use `size_bytes`).
- [ ] Remove package-wide loggers; transport constructors such as `ssh.New(cfg, logger)` now require an explicit `*zap.Logger` parameter.
- [ ] Replace any remaining `fmt.Print*` calls with structured `zap` logs to keep progress output fully structured.
- [ ] device: expand mountinfo parsing to handle bind mounts and multiple entries.
- [ ] Review the QUIC transport constructor and expand tests for both sender and receiver paths.
- [ ] LVM device support: plumb snapshot creation through the device abstraction, allow raw device fallbacks, and unit test LVM vs. file paths.
 - [x] Transport logging: emit connection handshake and teardown events with `snake_case` fields and ensure callers `defer rootcmd.SyncLogger(logger)`.
- [ ] Manifest rebuild: add a subcommand to regenerate chunk digests when manifests are missing or out of date, exercising rebuild logic in tests.
- [ ] Verify command: compare source and destination devices against manifest entries and surface mismatched digests with structured logs.
- [x] Manifest: return error when block size is 0 during creation.
- [x] Expand unit test coverage for remote execution and client signal handling, and run coverage reports (transports coverage ≥50%).

  ```sh
  go test -cover ./...
  ```

- [x] Run lint checks to keep style and correctness issues from creeping in for transports.

```sh
golangci-lint run
```

- [x] Add precedence tests for `ssh_host_key_path` flag, environment variable, and YAML configuration (see `internal/config/ssh_host_key_path_test.go`).

- [x] Ensure SSH agent connections use context timeouts and cover SSHManager reuse in tests.
- [x] common: add endianness mismatch handshake validation test.

- [ ] Add privilege escalation tests covering success and error paths (tests require root; skipped otherwise).
- [ ] Expand coverage for configuration precedence across flags, environment variables, and config files.
- [ ] Keep README configuration examples and precedence tests in sync.
- [ ] Keep modules single-purpose; maintain the `transfer` package decomposition (`progress.go`, `handshake.go`, `block_writer.go`, `resume.go`, `worker.go`, `writer.go`, `apply.go`, `transfer.go`). Future contributions should keep files small and focused.
- [ ] Keep `README` configuration documentation current with code changes.
- [ ] Keep transport documentation and configuration references (flags and env vars) up to date.
- [ ] Track decomposition of large files like `transfer/transfer.go`.
- [ ] Ensure progress logging uses `zap` exclusively.
- [ ] Ensure every new function has a dedicated unit test.
- [ ] Run `go build ./...`, `go test -cover ./...`, and `golangci-lint run` before merging changes.
- [ ] Maintain tests for compression detection, ensuring benchmark and cache logic remain correct.
- [ ] Add end-to-end tests for resume and verify workflows.
- [ ] Document flag grouping patterns and expand coverage for pflag/viper bindings.
- [ ] Maintain tests for buffer alignment, hole punching, and NUMA pinning.
- [ ] Enforce modular, single-responsibility design across packages.
- [ ] Document each new CLI flag, environment variable, and configuration option in `README.md`.
- [x] Refactor `main.go` into smaller modules.
- `cmd/dump` handles snapshot dumping and transport selection, receiving configuration and loggers explicitly.
- `cmd/root` configures the application and wires `cmd/dump`.
- [ ] Verify configuration precedence.
- [ ] Maintain tests for dedup configuration precedence and error paths (invalid YAML, parse errors).
- [ ] Keep README dedup configuration options in sync with code.
- [ ] Document feature changes in `README.md`.
- [x] Implement full transport registry with working QUIC, HTTP/2, TCP+TLS, and SSH backends and accompanying tests.
- [x] Finalize hybrid deduplication and document CDC tuning knobs.
- [x] Integrate adaptive compression sampling with configurable thresholds and unit benchmarks.
- [x] Fail fast when unsupported transports are requested and document `--transport` as reserved.
- [x] Add validation for CDC chunker parameters and corresponding tests.
  - Covered by `config/validation.go` tests.
- [ ] Ensure all commands default to a non-nil `zap.Logger` (use `zap.NewNop()`), eliminating nil checks.
## Roadmap

- [x] Introduce pluggable data plane with transport registry supporting QUIC, HTTP/2, TLS/TCP, and SSH.
- [x] Add hybrid deduplication combining fixed-size and content-defined chunking (FastCDC).
- [x] Implement adaptive compression with CPU feature detection and per-chunk sampling.
- [ ] Optimize transfer pipeline for concurrency autotuning, large in-flight windows, and efficient I/O paths.
- [ ] Provide `FlagSet`-grouped CLI options bound to Viper, with parity across flags, environment variables, and config files.
- [ ] Ensure each new function includes unit tests and documentation updates in README.md.
- [ ] Maintain modular, single-responsibility design to ease future maintenance.

## TODO

### Coding Conventions for New Packages
- [ ] Document file layout, naming, and single-purpose design for new packages.

### Testing Expectations
- [ ] Include unit tests for success and failure paths.
- [ ] Run `go build ./...`, `go test -coverprofile=coverage.out ./...`, and `golangci-lint run` before submitting patches.

### Logging Rules
- [ ] Use zap for structured logging with snake_case fields.
 - [ ] Call `rootcmd.SyncLogger(logger)` on shutdown and avoid `fmt.Print*` for progress output.
