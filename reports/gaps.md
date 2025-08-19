| Type | Component | Evidence | Impact | Fix | Priority |
|------|-----------|----------|--------|-----|----------|
| test | snapshot exhaustion | snapshot pressure handled only via mocks | real snapshot exhaustion may behave differently | add integration test covering snapshot exhaustion cleanup | medium |
<!-- no outstanding gaps -->
<!-- {"status": "closed", "component": "wal", "issue": "missing crash recovery docs and tests", "resolution": "added docs/wal.md and transfer/wal_crash_test.go"} -->
<!-- resolved: missing negative test for NUMA fallback covered -->
