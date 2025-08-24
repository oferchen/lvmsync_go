| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|
| Bug | Manifest | `go run /tmp/manifest_fail.go` → `manifest: file too small` | Corrupt manifests halt resume operations | Add rebuild command to regenerate or replace damaged manifests | High |
| Bug | RecvAck | `go test ./remote -run TestRecvAckPipe` → `RecvAck blocked beyond deadline` | Hung transfers and goroutine leaks when helper stdout lacks deadline support | Close reader or avoid draining when context is canceled; add tests | Medium |
