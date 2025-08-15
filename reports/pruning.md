| Path | Why removable | Evidence | Impact | Safe removal steps |
|------|---------------|---------|--------|--------------------|
| N/A | No prune candidates identified by `go vet`, `go test`, or `golangci-lint` | N/A | N/A | N/A |

## Regenerating prune candidates

Run the reachmap tool to refresh `.prune_candidates.txt`:

```sh
go run ./cmd/reachmap > .prune_candidates.txt
```
