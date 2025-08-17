| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|
| Bug | CLI flags | inconsistent flag naming across commands | confusing user experience | normalize flag names to kebab-case and document consistent style | medium |
| Bug | Privilege escalation | escalation command lacks timeout | operations may hang indefinitely | add context-aware timeout to privilege escalation | high |
