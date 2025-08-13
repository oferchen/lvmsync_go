| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|
| deprecated-api | grpc/client/client.go | used grpc.DialContext deprecated API | future gRPC releases could break dialing | replaced with grpc.NewClient | P1 |
| unreachable | internal/transport | package unused and failing vet | dead code increased maintenance | removed package and callers | P1 |
| doc-missing | README.md | `--transport` flag documented but not implemented | users misled about transport selection | docs note flag is ignored | P2 |
| bug | remote/remote.go | `setupHostKeyCallback` ignored verify flag | users could not disable host key verification | respect flag and use `ssh.InsecureIgnoreHostKey` when false | P1 |
| bug | remote/ssh_manager.go | `sshAgentAuth` used `net.Dial` without context or timeout | unresponsive SSH agent could hang start-up | added context-aware dial with timeout and tests | P1 |
