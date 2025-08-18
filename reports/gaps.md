| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|
| TODO | device | mountinfo parsing lacks bind mount and multiple-entry handling | device detection may fail on complex mounts | expand mountinfo parsing to handle bind mounts and multiple entries; add tests | medium |
| TODO | escalate | privilege escalation lacks test coverage | escalation regressions may go unnoticed | add root-only tests for privilege escalation success and failure paths | medium |
