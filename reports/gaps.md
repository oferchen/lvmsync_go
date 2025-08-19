| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|
| Test | escalate | EnsureRootOrReexec lacks root-level test of sudo allow list | Regressions could slip past CI | Add integration test executing sudo under root to verify allow list | medium |
