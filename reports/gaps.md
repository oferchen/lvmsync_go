| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|

| Testing | Privilege Escalation | AGENTS.md TODO: Add privilege escalation tests covering success and error paths | Privilege escalation logic untested; potential regressions or security issues | Implement privilege escalation tests for success and error paths; skip when non-root | P1 |
| Testing | Configuration Precedence | AGENTS.md TODO: Expand coverage for configuration precedence across flags, environment variables, and config files | Configuration precedence behavior unverified; user options may not apply as expected | Add tests covering configuration precedence across flags, environment variables, and config files | P2 |
