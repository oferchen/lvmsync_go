# Deduplication Strategies

When `--dedup-strategy auto` is selected, lvmsync chooses a strategy based on
available resources and CPU capabilities.

| Condition | Selected strategy |
|-----------|------------------|
| Bloom filter fits in RAM | `bloom` |
| Doesn't fit, checksum acceleration available | `checksum` |
| Doesn't fit, no acceleration | `rolling_hash` |
