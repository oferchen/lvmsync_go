| Path | Why removable | Evidence | Impact | Safe removal steps |
|------|---------------|---------|--------|--------------------|
| cmd/grpcd | unused gRPC daemon command with broken tests | go vet compile errors | simpler binary; faster tests | `git rm -r cmd/grpcd` (restore: `git checkout -- cmd/grpcd`) |
| internal/transport | package unused and contained merge conflicts | only imported by removed selectTransport | less dead code | `git rm -r internal/transport` (restore: `git checkout -- internal/transport`) |
| transfer/compression_test.go | test file malformed and failing vet | go vet syntax error | clean test run | `git rm transfer/compression_test.go` (restore: `git checkout -- transfer/compression_test.go`) |
| cmd/serve | stubbed serve command; no implementation | `serve mode not implemented` error | lighter CLI surface | `git rm -r cmd/serve` (restore: `git checkout -- cmd/serve`) |

## Regenerating prune candidates

Run the reachmap tool to refresh `.prune_candidates.txt`:

```sh
go run ./cmd/reachmap > .prune_candidates.txt
```

