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

## Dictionary Training

Early chunks seed a dictionary which is reused for subsequent blocks. The dictionary is trained from sampled data so small or repetitive chunks compress efficiently. Both LZ4 and Zstd dictionaries are supported.

## Long‑Distance Matching

Zstd enables long‑distance matching to reference data up to several megabytes back. This improves ratios when similar structures are separated by zeroed or unrelated regions.

## CPU Feature Detection

The `golang.org/x/sys/cpu` package detects SIMD extensions at runtime. LVMSync uses this information to choose the fastest implementation and to decide when Zstd is viable.

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
