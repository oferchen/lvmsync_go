# Summary
- Removed unused gRPC daemon and internal transport packages.
- Fixed compression algorithm selection and deprecated gRPC dial API.
- Updated documentation to reflect current transport support.

## Risks
- Transport selection remains unimplemented; future work needed for real transports.

## Next Steps
- Implement transport backends and reintroduce comprehensive compression tests.
