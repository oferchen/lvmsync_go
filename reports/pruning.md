| Path | Why removable | Evidence | Impact | Safe removal steps |
|------|---------------|---------|--------|--------------------|
| cmd/grpcd | unused gRPC daemon command with broken tests | go vet compile errors | simpler binary; faster tests | `git rm -r cmd/grpcd` (restore: `git checkout -- cmd/grpcd`) |
| internal/transport | package unused and contained merge conflicts | only imported by removed selectTransport | less dead code | `git rm -r internal/transport` (restore: `git checkout -- internal/transport`) |
| transfer/compression_test.go | test file malformed and failing vet | go vet syntax error | clean test run | `git rm transfer/compression_test.go` (restore: `git checkout -- transfer/compression_test.go`) |
