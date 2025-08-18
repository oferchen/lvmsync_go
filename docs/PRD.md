# LVMSync PRD

## Overview

LVMSync provides high-performance block-level replication for LVM snapshots on 64-bit Linux systems. The repository already includes Go modules, basic tests and CI, but still needs improved coverage, error handling and developer documentation. This PRD outlines remaining capabilities, coding standards and tasks for future development.

## Supported Platforms

LVMSync runs on Linux only and is validated on the `amd64` and `arm64` architectures.

## Goals

- Reliable incremental replication of LVM snapshots across hosts.
- Resume and deduplicate transfers to minimize bandwidth usage.
- Easy to operate locally or over SSH with configurable options.

## lvmsyncd Architecture

The `lvmsyncd` daemon loads modules and listens on URIs specified by `--listen`.
Each scheme selects a transport and configuration is sourced from flags,
`LVMSYNC_DAEMON_*` environment variables, or a `lvmsyncd.yaml` file. The flag
surface remains minimal and all logging uses the structured `zap` logger.


## Identified Gaps

1. **Deduplication State Loading** – deduplication strategies save state but never load existing state files on startup.
2. **Limited Test Coverage** – initial tests exist but many edge cases remain uncovered.
3. **Error Handling in Remote Operations** – remote SSH connections lack retry logic and clearer error messages for network failures.
4. **Documentation and Architecture Overview** – developer guidelines and package-level architecture descriptions are still sparse.

## Coding Guidelines

- Use `go fmt` and `go vet` to maintain code style and catch common mistakes.
- Prefer explicit error handling; log contextual details using `zap`.
- Keep functions small and focused; avoid side effects where possible.
- Favor Go standard library functionality; introduce external dependencies only when well supported and necessary.
- Interact with LVM using maintained Go bindings to `liblvm2cmd` instead of shelling out.
- Protect concurrent access to shared state with mutexes.
- Maintain backward compatibility for CLI flags when adding new features.

## Proposed Tasks

1. **Load Deduplication State**
   - Implement a function that loads existing deduplication information from the configured state file during startup.
   - Provide unit tests verifying dedup behaviour across runs.

2. **Expand Testing Infrastructure**
   - Add Go unit tests for packages in `lvm`, `remote` and `transfer`.
   - Cover edge cases such as snapshot creation failures and checksum mismatches.

3. **Improve Remote Error Handling**
   - Add retry logic for establishing SSH connections.
   - Surface clearer errors when remote commands fail.
4. **Refactor Monolithic Functions**
   - Break down large functions (e.g., client modes) into smaller composable units to improve readability and testability.

5. **Extended Documentation**
   - Document package architecture and developer guidelines.
   - Explain how to run tests and CI locally.

6. **Document lvmsyncd Architecture**
   - Describe module loading, listen URIs, and configuration precedence.

Contributors tackling these tasks must run:

```sh
go build ./...
go test -cover ./...
golangci-lint run
```

## Acceptance Criteria

- Deduplication state persists across runs and reduces transferred blocks when repeated.
- Tests provide broad coverage of `lvm`, `remote` and `transfer` packages.
- CI pipeline executes `go vet`, `go fmt`, and `go test ./...` on every pull request.
- Documentation clearly describes modular architecture, setup, contributing and coding conventions.
