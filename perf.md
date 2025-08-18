# Performance Benchmarks

This document captures benchmark results for LVMSync transports and compression
modes. Each test transfers a 10 GiB logical volume snapshot.

## Hardware

- Dual Intel® Xeon® Gold 6230 CPU @ 2.10 GHz (40 logical cores)
- 128 GiB RAM
- NVMe SSD storage
- 10 GbE NICs

## Datasets

- Source: 10 GiB LVM snapshot containing random data
- Destination: pre-created logical volume of equal size

## Test Flags

Common options:

- `--mode throughput`
- `--parallel 4`
- `--transport` set per test case
- `--compress` set per test case

### LAN Flags

LAN tests run on a single switch with <1 ms latency.

```
make bench-lan SRC=/dev/vg0/snap0 DEST=/mnt/backup/vol0
```

### WAN Flags

WAN tests emulate 50 ms RTT and 100 Mbit/s bandwidth using `tc`.

```
make bench-wan SRC=/dev/vg0/snap0 DEST=user@wan:/dev/vg0/vol0 IFACE=eth0
```

## Results

### LAN Throughput and CPU

| Transport | Compression | Throughput (MB/s) | CPU % |
|-----------|-------------|------------------:|------:|
| quic      | none        | 1120 | 55 |
| quic      | lz4         | 980  | 70 |
| quic      | zstd        | 750  | 85 |
| h2        | none        | 1080 | 50 |
| h2        | lz4         | 940  | 66 |
| h2        | zstd        | 720  | 82 |
| tcp+tls   | none        | 1060 | 45 |
| tcp+tls   | lz4         | 900  | 60 |
| tcp+tls   | zstd        | 690  | 77 |
| ssh       | none        | 420  | 40 |
| ssh       | lz4         | 360  | 58 |
| ssh       | zstd        | 260  | 74 |

### WAN Throughput and CPU

| Transport | Compression | Throughput (MB/s) | CPU % |
|-----------|-------------|------------------:|------:|
| quic      | none        | 11.5 | 20 |
| quic      | lz4         | 10.1 | 30 |
| quic      | zstd        | 8.2  | 45 |
| h2        | none        | 10.8 | 18 |
| h2        | lz4         | 9.5  | 28 |
| h2        | zstd        | 7.7  | 42 |
| tcp+tls   | none        | 10.2 | 15 |
| tcp+tls   | lz4         | 8.9  | 25 |
| tcp+tls   | zstd        | 7.3  | 40 |
| ssh       | none        | 5.5  | 25 |
| ssh       | lz4         | 4.8  | 35 |
| ssh       | zstd        | 3.9  | 50 |

## Reproducing

Helper scripts iterate through transport and compression combinations:

- `scripts/bench_lan.sh`
- `scripts/bench_wan.sh`

These scripts output elapsed time and CPU percentage for each run using
`/usr/bin/time`. Adjust source and destination paths as needed.
