# Write-Ahead Log

LVMSync uses a write-ahead log (WAL) to record applied block ranges so interrupted transfers can safely resume.

## Header Layout

The first 112 bytes contain fixed metadata:

| Offset | Length | Field      | Description                 |
|--------|--------|------------|-----------------------------|
| 0      | 8      | size       | Total device size in bytes  |
| 8      | 8      | epoch      | Transfer epoch              |
| 16     | 64     | device_id  | UTF-8 identifier, zero padded |
| 80     | 32     | mac        | BLAKE3-256 of previous fields |

Each subsequent entry is 16 bytes and encodes a completed range as little-endian `start` and `end` offsets.

## Fsync Requirements

`OpenWAL` writes the header and fsyncs both the WAL and its parent directory when the file is first created. `Append` writes a 16-byte entry and fsyncs the file before returning. `Close` fsyncs the file and its parent directory to guarantee durability. Callers that bypass these fsyncs risk losing entries after power loss.

## Replay Semantics

On startup `OpenWAL` validates the header MAC and metadata. It scans entries 16 bytes at a time, truncating any partial tail left by a crash. Only fully written entries are returned for replay. Entries that were written without a matching fsync may be lost after power failure.

