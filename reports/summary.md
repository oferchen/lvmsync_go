# Summary
- Removed unused gRPC daemon and internal transport packages.
- Fixed compression algorithm selection and deprecated gRPC dial API.
- Updated documentation to reflect current transport support.
- Fixed SSH host key verification flag handling.
- Added timeout-aware SSH agent authentication and tests verifying SSH client reuse.
- SelectTransport now errors when unsupported transports are requested and documentation reflects reserved status.

## Risks
- Transport selection remains unimplemented; implement real transports to replace the current error.

## Next Steps
- Implement transport backends and reintroduce comprehensive compression tests.
