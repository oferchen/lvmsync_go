# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Privilege escalation tests covering sudo success and failure modes.
- device: tests for GetUUID and IsMountedRW

### Fixed
- validate block size mismatch in handshakes

## [v0.1.0] - 2025-02-27
### Added
- Initial release of LVMSync with a pluggable transport registry supporting QUIC, HTTP/2, TCP+TLS, and SSH.
- Hybrid deduplication combining fixed-size and content-defined chunking.
- Adaptive compression with CPU feature detection and per-chunk sampling.
- gRPC control plane with mutual TLS and keepalive support.
- Throughput-optimized transfer mode.

### Fixed
- N/A
