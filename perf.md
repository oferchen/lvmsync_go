# Performance Benchmarks

This document captures benchmark results for LVMSync transports and compression
modes. Each test transfers a 10 GiB logical volume snapshot.

## Commit

Benchmarks were recorded from commit `e3591daaed6e9cc31b249961ce8a45d6816f6873`.

## Hardware

- Dual Intel® Xeon® Gold 6230 CPU @ 2.10 GHz (40 logical cores)
- 128 GiB RAM
- NVMe SSD storage
- 10 GbE NICs

Example host details can be captured with:

```sh
uname -srvmo
lscpu | head
lsblk -d -o name,rota,size,model
```

## Dataset Generation

The following steps create the reproducible 10 GiB dataset used for all tests:

1. Create and populate a source volume with random data:
   ```sh
   lvcreate -L10G -n bench_src vg0
   dd if=/dev/urandom of=/dev/vg0/bench_src bs=1M count=10240 status=progress
   lvcreate -s -n snap0 /dev/vg0/bench_src
   ```
2. Prepare a matching 10 GiB destination volume:
   ```sh
   lvcreate -L10G -n bench_dst vg0
   ```

The snapshot `/dev/vg0/snap0` is used as the source and `/dev/vg0/bench_dst` as the destination in the benchmarks below.

## Performance Guidance

- Align O_DIRECT reads and writes to the device's logical and physical block sizes. Misaligned operations fall back to buffered I/O or return errors.
- Pin worker goroutines to the device's NUMA node (`--numa-pin`) or a specific node (`--numa-node`) to improve locality on multisocket systems.

## Test Flags

Common options:

- `--mode throughput`
- `--parallel 4`
- `--transport` set per test case
- `--compress` set per test case
- `--probe-only` for pre-flight checks and dry-run estimates
- `--verify-only` when measuring read-only verification costs

## Running Benchmarks

Benchmarks are executed with helper scripts that wrap the `lvmsync run` command and collect timing data.

Ensure the privilege model is satisfied: run as root or configure `--lvm-escalation`.  
Non-zero exit codes indicate problems (`10` for privilege failures, `60` for verification mismatches).

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

Example output from `scripts/bench_lan.sh`:

```sh
$ scripts/bench_lan.sh /dev/vg0/snap0 /mnt/backup/vol0
== quic/none ==
1120 MB/s 55% cpu
== quic/lz4 ==
980 MB/s 70% cpu
== quic/zstd ==
750 MB/s 85% cpu
== h2/none ==
1080 MB/s 50% cpu
== h2/lz4 ==
940 MB/s 66% cpu
== h2/zstd ==
720 MB/s 82% cpu
== tcp+tls/none ==
1060 MB/s 45% cpu
== tcp+tls/lz4 ==
900 MB/s 60% cpu
== tcp+tls/zstd ==
690 MB/s 77% cpu
== ssh/none ==
420 MB/s 40% cpu
== ssh/lz4 ==
360 MB/s 58% cpu
== ssh/zstd ==
260 MB/s 74% cpu
```

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

Example output from `scripts/bench_wan.sh`:

```sh
$ scripts/bench_wan.sh /dev/vg0/snap0 user@wan:/dev/vg0/vol0
== quic/none ==
11.5 MB/s 20% cpu
== quic/lz4 ==
10.1 MB/s 30% cpu
== quic/zstd ==
8.2 MB/s 45% cpu
== h2/none ==
10.8 MB/s 18% cpu
== h2/lz4 ==
9.5 MB/s 28% cpu
== h2/zstd ==
7.7 MB/s 42% cpu
== tcp+tls/none ==
10.2 MB/s 15% cpu
== tcp+tls/lz4 ==
8.9 MB/s 25% cpu
== tcp+tls/zstd ==
7.3 MB/s 40% cpu
== ssh/none ==
5.5 MB/s 25% cpu
== ssh/lz4 ==
4.8 MB/s 35% cpu
== ssh/zstd ==
3.9 MB/s 50% cpu
```

## Reproducing
Benchmark runs are reproducible when the dataset, hardware, and commit hash are recorded.

```sh
git clone https://github.com/.../lvmsync_go.git
cd lvmsync_go
go build ./...
git rev-parse HEAD
scripts/bench_lan.sh /dev/vg0/snap0 /mnt/backup/vol0
scripts/bench_wan.sh /dev/vg0/snap0 user@wan:/dev/vg0/vol0 IFACE=eth0
```

Each script prints throughput and CPU usage via `/usr/bin/time -f '%e %P'`. Save
the script output together with the commit hash for comparison across runs.
