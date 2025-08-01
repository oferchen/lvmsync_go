# LVMSync PRD

## Overview
LVMSync provides high-performance block-level replication for LVM snapshots. The current repository contains functional code but lacks automated tests, CI, and some robustness features. This PRD outlines missing capabilities, coding standards and tasks for future development.

## Goals
- Reliable incremental replication of LVM snapshots across hosts.
- Resume and deduplicate transfers to minimize bandwidth usage.
- Easy to operate locally or over SSH with configurable options.

## Identified Gaps
1. **Module and Dependency Management** – repository lacks a `go.mod` file, making dependency management and builds unreliable.
2. **Deduplication State Loading** – deduplication strategies save state but never load existing state files on startup.
3. **Automated Tests** – no unit or integration tests exist for the packages.
4. **Continuous Integration** – no CI workflow is configured to ensure formatting, linting or tests pass.
5. **Error Handling in Remote Operations** – remote SSH connections lack retry logic and clearer error messages for network failures.
6. **Documentation** – no developer guidelines or contribution process other than a short note in the README.

## Coding Guidelines
- Use `go fmt` and `go vet` to maintain code style and catch common mistakes.
- Prefer explicit error handling; log contextual details using `zap`.
- Keep functions small and focused; avoid side effects where possible.
- Protect concurrent access to shared state with mutexes.
- Maintain backward compatibility for CLI flags when adding new features.

## Proposed Tasks
1. **Introduce Go Modules**
   - Add `go.mod` and `go.sum` files listing all dependencies.
   - Update import paths if necessary to use module names.

2. **Load Deduplication State**
   - Implement a function that loads existing deduplication information from the configured state file during startup.
   - Provide unit tests verifying dedup behaviour across runs.

3. **Testing Infrastructure**
   - Add Go unit tests for packages in `lvm`, `remote` and `transfer`.
   - Cover edge cases such as snapshot creation failures and checksum mismatches.

4. **Continuous Integration**
   - Configure GitHub Actions to run `go vet`, `go test ./...` and `go fmt` checks.
   - Fail the build when formatting or tests do not pass.

5. **Improve Remote Error Handling**
   - Add retry logic for establishing SSH connections.
   - Surface clearer errors when remote commands fail.

6. **Extended Documentation**
   - Expand the README with build instructions using modules.
   - Document coding guidelines and how to run tests and CI locally.

## Acceptance Criteria
- Repository builds successfully via `go build ./...` with modules enabled.
- Unit tests cover main functionality with at least 80% package coverage.
- CI pipeline executes on every pull request and enforces formatting and tests.
- Deduplication state persists across runs and reduces transferred blocks when repeated.
- Documentation clearly describes setup, contributing and coding conventions.
