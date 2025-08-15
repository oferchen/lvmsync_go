# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Log dial and listen lifecycle events with duration_ms and error fields across transports, with tests covering handshake and teardown logs.
- Log final handshake parameters for all transports.
- Log manifest rebuild completion with size and duration metrics.
- Add handshake parsing tests for unknown and malformed tokens.
- Bind LVMSYNC_DEDUP_* environment variables and extend tests for dedup flag precedence.
- Privilege escalation tests covering sudo success and failure modes.
- device: tests for GetUUID and IsMountedRW
- device: persist thaw command configuration for raw devices
- transport/h2: refactor Dial into dialTLS, performH2Handshake, and logDialResult with unit tests.
- transport: tests covering SelectBest handshake negotiation with custom CDC settings, resume tokens, and O_DIRECT for ssh, tcp+tls, h2, and quic transports.
- device: add raw device freeze/thaw tests with exec command stubs
- transfer: add manifest index persistence test covering read/write, rebuild, and verify paths.
- cmd: support `--source-type` and `--dest-type` flags and allow `device.Detect` to honor explicit type hints.
- transfer: unify resume checkpoints across dedup modes and add resume tests for fixed, CDC, and hybrid modes.
- manifest: add `manifest_timeout` option to control rebuild timeout.
- device: allow configurable LVM privilege escalation command.
- tests: verify ALPN and TLS version round-trip in handshake and transport negotiation.
- manifest: test zero `manifest_timeout` uses background context.
- cmd/verify: add test ensuring mismatched blocks log `mismatched_block`.
- README: document CDC parameter ordering and the error when violated.
- README: note that commands accept an explicit `*zap.Logger` defaulting to `zap.NewNop()`.


### Fixed
- tests: assert context deadline exceeded for tcp+tls unreachable dial
- Enforce CDC chunk size ordering in handshake validation.
- validate block size mismatch in handshakes
- dedup: validate chunker size parameters.
- remove placeholder error field from dial_start and listen_start logs for h2 and tcp+tls transports
- propagate seek errors during block writes to prevent silent data loss
- Remove obsolete gap and pruning entries after rerunning static analysis.
- Wrap README logging example in `package main` to compile with `go build`.
- add default dial timeout and deadline propagation for h2 and tcp+tls transports
- reduce tcp+tls default dial timeout to 5s
- close SSH agent connections after retrieving signers and apply context-based timeouts
- shorten h2 dial timeout to 5s and enforce it when no deadline is provided
- ensure verify and lvmsync commands flush logs by deferring logger.Sync()
- remove legacy dedup manifest in favor of manifest.Index
- device: surface freeze/thaw command output when raw device operations fail
- transfer: give each writer an independent rate limiter
- cmd/dump: handle context cancellation during pipe copies to avoid incomplete writes
- rename dry run log field to `eta_seconds` and log durations in seconds
- transfer: replace global checkpoint state with per-transfer resume trackers
- cmd/dump: detect destination type locally without mutating configuration
- manifest: propagate close errors when rebuilding indexes
- log sync errors in manifest and verify commands
- h2: ensure unreachable dial test uses context timeout and expects deadline exceeded

## [v0.1.0] - 2025-02-27
### Added
- Initial release of LVMSync with a pluggable transport registry supporting QUIC, HTTP/2, TCP+TLS, and SSH.
- Hybrid deduplication combining fixed-size and content-defined chunking.
- Adaptive compression with CPU feature detection and per-chunk sampling.
- gRPC control plane with mutual TLS and keepalive support.
- Throughput-optimized transfer mode.

### Fixed
- N/A
