# Verify

`lvmsync verify` compares two devices and reports any blocks that differ.
It reads both paths in chunks, computing BLAKE3 digests for each block. A
non-zero exit status is returned when mismatches are detected.

## Examples

Verify that two logical volumes match:

```sh
lvmsync verify /dev/vg0/source /dev/vg1/target
```

Restrict verification to ranges recorded in a manifest and use 4 KiB blocks:

```sh
lvmsync verify --block_size 4K --manifest_path snapshot.manifest /dev/vg0/source /dev/vg1/target
```

All flags can also be provided via `LVMSYNC_*` environment variables or a
`config.yaml` file thanks to Viper binding.
