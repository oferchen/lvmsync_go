# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- docs: document snapshot cleanup, resume, and verify-only rollback procedures in docs/danger_rollback.md
- device: split Detect into detectFileDevice, detectLVMDevice, and detectRawDevice helpers with dedicated tests
- device: detect LVM, raw, or file devices using blkid metadata
- device: enforce snapshot requirement for LVM volumes unless --offline is set
- cmd: manage LVM snapshot lifecycle within dump and apply commands
- tests: add coverage for adaptive chunk sizing, index option application, TLS version helper, capability checks, and data size overflow; include loop device setup integration test.
- Log dial and listen lifecycle events with duration_ms and error fields across transports, with tests covering handshake and teardown logs.
- ssh: log connection close events with address, role, and duration fields.
- Log final handshake parameters for all transports.
- Log manifest rebuild completion with size and duration metrics.
- Add handshake parsing tests for unknown and malformed tokens.
- Bind LVMSYNC_DEDUP_* environment variables and extend tests for dedup flag precedence.
- ssh: allow specifying host key path and load host keys from disk
- Bind LVMSYNC_COMPRESSION_* environment variables and add tests for compression flag precedence.
- Privilege escalation tests covering sudo success and failure modes.
- device: tests for GetUUID and IsMountedRW
- device: persist thaw command configuration for raw devices
- device: split OpenRaw into prepareFreeze, openDevice, and queryDeviceInfo helpers with targeted tests
- transport/h2: refactor Dial into dialTLS, performH2Handshake, and logDialResult with unit tests.
- transfer: block writer with sync interval and optional checksum verification; tests for fdatasync intervals and MAC mismatches
- transport: tests covering SelectBest handshake negotiation with custom CDC settings, resume tokens, and O_DIRECT for ssh, tcp+tls, h2, and quic transports.
- transport: test registry fallback dialing sequence with logged attempts.
- common: add MergeHandshake helper for compressor and digest negotiation.
- common: add handshake validation test for endianness mismatches.
- device: add raw device freeze/thaw tests with exec command stubs
- device: centralize exec command helper for LVM and raw devices
- device: add cleanup tests for thaw errors and timeouts
- transfer: add manifest index persistence test covering read/write, rebuild, and verify paths.
- transfer: persist resume checkpoints with `--checkpoint-bytes` and `--checkpoint-interval` and add tests for interrupted transfers.
- cmd: support `--source-type` and `--dest-type` flags and allow `device.Detect` to honor explicit type hints.
- transfer: unify resume checkpoints across dedup modes and add resume tests for fixed, CDC, and hybrid modes.
- transfer: add tests verifying resume state alignment across dedup mode transitions.
- manifest: add `manifest_timeout` option to control rebuild timeout.
- device: allow configurable LVM privilege escalation command.
- tests: verify ALPN and TLS version round-trip in handshake and transport negotiation.
- tests: enforce ALPN and TLS version mismatches across transports.
- manifest: test zero `manifest_timeout` uses background context.
- cmd/verify: add test ensuring mismatched blocks log `mismatched_block`.
- tests: cover manifest precedence for path, timeout, and allow_mounted.
- Document tasks for CDC chunker parameter validation and default logger initialization in `AGENTS.md`.
- transport/h2: add tests for tlsVersionString and roleString helpers.
- README: document CDC parameter ordering and the error when violated.
- device: compare device identifiers via LVM LV UUID or blkid/PARTUUID
- remote: validate privileged helper command names against `^[a-zA-Z0-9._-]+$`
- privilege: ensure non-interactive sudo escalation via `--lvm-escalation` flag with validation.
- README: note that commands accept an explicit `*zap.Logger` defaulting to `zap.NewNop()`.
- tests: validate O_DIRECT mismatch and agreement in handshake validation.
- transport: add Role.String and shared TLSVersionString helpers.
- manifest: unify index options for device detection and close hooks across constructors.
- manifest: consolidate option handling and add tests for custom DetectDevice and CloseHook.
- manifest: use functional IndexOption for device detection and close hooks.
- hash: add Blake3Hasher tests covering keyed and unkeyed modes with state reset verification.
- transfer: tests ensure mismatched device UUIDs and mounted devices return errors without writes.
- cmd/verify: support SHA-256 verification with configurable digest helper.
- transport: centralize handshake logging via `HandshakeFields` helper.
- manifest: allow custom close hook via index options, removing global hook.
- Document manifest usage, transport security defaults, and resume/verify workflows.
- compressiondetect: choose zstd when AVX2 or NEON is available.
- transfer: sample 8 KiB per chunk and log compression decisions.
- config: warn on unused or unknown keys in YAML config files.
- ci: build and smoke-test distroless release image.

### Fixed
- lvm: parse `--lvm-escalation` using shell-style quoting and reject unsafe formats
- tests: accept net.Error timeouts in h2 handshake timeout
- manifest: log configuration warnings with structured fields
- lock: reject VG/LV names with invalid characters via regex validation
- tests: flush H2 handshake data by buffering server errors, closing connections before sending, and applying context timeouts
- manifest: return error when block size is zero
- device: Detect, OpenFile, and OpenRaw return error when logger is nil
- device: parse mount info using github.com/moby/sys/mountinfo to handle spaces and special characters
- transport: return "unknown" or numeric string for unrecognized TLS versions
- config: remove duplicate prefixes in LVMSYNC_DEDUP_* environment bindings.
- device: require --offline or freeze/thaw hooks for raw devices
- device, transfer: require non-nil loggers and remove conditional logging
- config: avoid warnings for alias keys in YAML (e.g., allow-insecure vs. allow_insecure)
- tests: fix O_DIRECT match and transport mismatch handshake validation tests
- quic: remove redundant deadline methods and use Role.String for dial/listen
- device: remove logger nil guards and default to `zap.NewNop()`
- quic: propagate deadlines to connection for datagram reads.
- tests: assert context deadline exceeded for tcp+tls unreachable dial
- handshake: validate digest mismatches in protocol negotiation
- Enforce CDC chunk size ordering in handshake validation.
- tests: add timeouts and connection cleanup in tcp+tls handshake selection test.
- config: validate positive CDC tunables and ordering.
- tcp+tls: ensure negotiation performs TLS handshake and only records ALPN/TLS version when negotiated.
- config: enforce CDC chunk size ordering during validation.
- validate block size mismatch in handshakes
- manifest: remove merge conflict markers and duplicate option definitions
- tests: restore transport mismatch check in handshake validation
- handshake: validate transport mismatch in handshakes
- dedup: validate chunker size parameters.
- config: use non-interactive sudo in validation tests to avoid user dependency
- remove placeholder error field from dial_start and listen_start logs for h2 and tcp+tls transports
- manifest: preserve injected close hooks in Create, Open, and Upgrade
- remove placeholder error field from dial_start and listen_start logs for quic and ssh transports
- propagate seek errors during block writes to prevent silent data loss
- manifest: record CDC chunk size parameters and hybrid flags in header and index entries
- cmd/manifest: add `manifest rebuild` subcommand for regenerating manifests
- config: expose `--cdc-min`, `--cdc-avg`, and `--cdc-max` flags bound to Viper
- docs: document manifest lifecycle and CDC options
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
- allow resuming transfers after changing dedup modes
- cmd/dump: handle context cancellation during pipe copies to avoid incomplete writes
- rename dry run log field to `estimated_duration_ms` and add `estimated_bandwidth_bps`
- transfer: replace global checkpoint state with per-transfer resume trackers
- tests: set LVMEscalation in CDC validation test
- cmd/dump: detect destination type locally without mutating configuration
- manifest: propagate close errors when rebuilding indexes
- log sync errors in manifest and verify commands
- ensure apply and dump commands flush logs with deferred SyncLogger
- h2: ensure unreachable dial test uses context timeout and expects deadline exceeded
- manifest: Rebuild defaults to `zap.NewNop()` and removes conditional logging checks
- device: reject freeze/thaw command paths with invalid characters and document allowed format
- device: allow freeze/thaw command paths containing directories by validating basename only
- dedup: ensure FastCDC chunks own their data slices to prevent cross-chunk mutation
- device: parse freeze/thaw commands with shell-style quoting

- manifest: fix close hook test to remove duplicate rebuild blocks
- tcp+tls: log listener close errors during shutdown
- tests: bound QUIC SelectBestHandshake dial and negotiate with timeouts and close server connection before reporting errors
- tests: use timeouts and close connections before returning errors in SSH handshake selection test
- ssh: aggregate channel close errors from client and server connections

## [v0.2.0] - 2025-08-22
### Added
- Refactored device detection into dedicated helpers for LVM, raw, and file volumes.
- Persisted resume checkpoints across dedup modes with configurable intervals.

### Fixed
- Validates privilege escalation commands and freeze/thaw configuration for raw devices.

### SBOM
- [Release SBOM](https://github.com/oferchen/lvmsync_go/releases/download/v0.2.0/lvmsync-v0.2.0-linux-amd64.sbom)

## [v0.1.0] - 2025-02-27
### Added
- Initial release of LVMSync with a pluggable transport registry supporting QUIC, HTTP/2, TCP+TLS, and SSH.
- Hybrid deduplication combining fixed-size and content-defined chunking.
- Adaptive compression with CPU feature detection and per-chunk sampling.
- Throughput-optimized transfer mode.

### Fixed
- N/A
