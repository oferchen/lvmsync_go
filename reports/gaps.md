| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|
| deprecated-api | grpc/client/client.go | used grpc.DialContext deprecated API | future gRPC releases could break dialing | replaced with grpc.NewClient | P1 |
| unreachable | internal/transport | package unused and failing vet | dead code increased maintenance | removed package and callers | P1 |
| doc-missing | README.md | `--transport` flag documented but not implemented | users misled about transport selection | docs note flag is ignored | P2 |
