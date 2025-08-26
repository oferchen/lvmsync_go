# Write-Ahead Log

LVMSync uses a write-ahead log (WAL) to record applied block ranges so interrupted transfers can safely resume.

## Header Layout

The first 120 bytes contain fixed metadata:

| Offset | Length | Field         | Description                          |
|--------|--------|---------------|--------------------------------------|
| 0      | 8      | version       | WAL format version (currently `2`)   |
| 8      | 8      | size          | Total device size in bytes           |
| 16     | 8      | epoch         | Transfer epoch                       |
| 24     | 64     | kernel_uuid   | Kernel-reported device UUID          |
| 88     | 64     | gpt_uuid      | GPT partition UUID                   |
| 152    | 8      | mbr_signature | MBR disk signature (hex)             |
| 160    | 64     | fs_uuid       | Filesystem UUID                      |
| 224    | 4      | major         | Device major number                  |
| 228    | 4      | minor         | Device minor number                  |
| 232    | 32     | mac           | BLAKE3-256 of the previous fields    |

Each subsequent entry is 16 bytes and encodes a completed range as little-endian `start` and `end` offsets.

## Version Semantics

New WALs default to version `2`. `OpenWAL` rejects headers with any other version.
WALs written by earlier releases are automatically upgraded to version `2` on
open, rewriting the header and preserving existing entries.

## Fsync Requirements

`OpenWAL` writes the header and fsyncs both the WAL and its parent directory when the file is first created. `Append` writes a 16-byte entry and fsyncs the file before returning. `Close` fsyncs the file and its parent directory to guarantee durability. Callers that bypass these fsyncs risk losing entries after power loss.

## Replay Semantics

On startup `OpenWAL` validates the header MAC and metadata. It scans entries 16 bytes at a time, truncating any partial tail left by a crash. Only fully written entries are returned for replay. Entries that were written without a matching fsync may be lost after power failure.

