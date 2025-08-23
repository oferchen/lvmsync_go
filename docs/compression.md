# Compression Pipeline

LVMSync compresses data on a per‑chunk basis. Each chunk passes through a classifier that samples 8 KiB of data and estimates the savings before compression is attempted.

## Chunk Classifier

1. **Sample** – 8 KiB from the chunk is compressed.
2. **Thresholds** – the estimated ratio is compared against `--compress-threshold` and the predicted byte reduction against `--min-savings`.
3. **Decision** – chunks meeting both thresholds are compressed; others are sent uncompressed.

`--compress-threshold` skips compression when the sample ratio is greater than or equal to the threshold (default `0.9`).
`--min-savings` requires at least the specified number of bytes saved before compression is attempted.

## Algorithm Selection

`--compress auto` chooses between LZ4 and Zstd for each chunk:

- Chunks <256 KiB use LZ4.
- Larger chunks use Zstd when the CPU reports AVX2 (amd64) or NEON (arm64) via [`x/sys/cpu`](https://pkg.go.dev/golang.org/x/sys/cpu); otherwise LZ4 is used.

Compression levels are tuned with `--zstd-level 1..5` or `--lz4-level {fast|hc}`.

### Selection Matrix

| Chunk size      | CPU features      | Algorithm | Level mapping                           |
|-----------------|-------------------|-----------|-----------------------------------------|
| `<256` KiB      | Any               | LZ4       | level1 when `--lz4-level` is `0`        |
| `≥256` KiB      | AVX2 or NEON      | Zstd      | default `1`; values `>3` cap to `3`     |
| `≥256` KiB      | Neither detected  | LZ4       | level1 when `--lz4-level` is `0`        |

## Dictionary Training

Early chunks seed a dictionary which is reused for subsequent blocks. The dictionary is trained from sampled data so small or repetitive chunks compress efficiently. Both LZ4 and Zstd dictionaries are supported.

## Long‑Distance Matching

Zstd enables long‑distance matching to reference data up to several megabytes back. This improves ratios when similar structures are separated by zeroed or unrelated regions.

## CPU Feature Detection

The `golang.org/x/sys/cpu` package detects SIMD extensions at runtime. LVMSync uses this information to choose the fastest implementation and to decide when Zstd is viable.

## Frame Format and Resumability

Each chunk is written as an independent frame so transfers can restart at chunk boundaries. The frame layout is:

| Offset | Length | Field     | Description                                                           |
|-------:|-------:|-----------|-----------------------------------------------------------------------|
| 0      | 8      | offset    | Byte position within the source device                                |
| 8      | 4      | length    | Uncompressed chunk size; `0` denotes a sparse range                   |
| 12     | 4      | crc32c    | CRC32C of the uncompressed data                                       |
| 16     | n      | checksum  | Optional BLAKE3/SHA‑256 digest when `--verify-checksum` is enabled    |
| 16+n   | m      | payload   | Compressed chunk data (omitted for sparse ranges)                     |

The `offset` field doubles as a resume marker. LVMSync records the last completed frame in the resume state so interrupted transfers can skip transmitted chunks without recompressing them.

### Resume Example

An aborted transfer can continue from the next frame boundary:

```sh
# Initial run interrupted after the first frame
lvmsync run --block-size=1M --resume state.json /dev/vg0/snap0 /dev/vgd/dest
# Resume sends only the remaining frames
lvmsync run --block-size=1M --resume state.json /dev/vg0/snap0 /dev/vgd/dest
```

Frames are self‑contained and carry their own checksums. When resuming, LVMSync verifies previously written frames and continues with subsequent ones without recompressing earlier data.

## Throughput Comparison

Integration tests stream a gzip-compressed block and a zero-filled block through the compressor. The pre-compressed block was sent unmodified, achieving ~5.3 MB/s, while the zero-filled block compressed to ~49.7 MB/s.

## Configuration Examples

### CLI

```sh
lvmsync run --compress auto --zstd-level 2 --lz4-level hc \
  --compress-threshold 0.85 --min-savings 65536 /dev/vg0/snap0 /dev/vg0/data
```

### Environment

```sh
LVMSYNC_COMPRESSION_COMPRESS=auto \
LVMSYNC_COMPRESSION_ZSTD_LEVEL=2 \
LVMSYNC_COMPRESSION_LZ4_LEVEL=hc \
LVMSYNC_COMPRESSION_COMPRESS_THRESHOLD=0.85 \
LVMSYNC_COMPRESSION_MIN_SAVINGS=65536 \
lvmsync run /dev/vg0/snap0 /dev/vg0/data
```

### YAML

```yaml
compress: auto
zstd_level: 2
lz4_level: hc
compress_threshold: 0.85
min_savings: 65536
```
