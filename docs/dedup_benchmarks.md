# Dedup Benchmarks

Benchmarks compare fixed-size, content-defined (CDC), and hybrid chunking modes
on patterned and random datasets using Go's testing framework.

## Parameters

- Dataset size: 1 MiB
- Patterned data: repeating 16 byte sequence
- Random data: pseudo-random bytes with deterministic seed
- Fixed block size: 64 KiB
- CDC sizes: 4 KiB min, 8 KiB avg, 16 KiB max
- Hybrid: fixed 64 KiB blocks with CDC on each block
- Command: `go test -run '^$' -bench Benchmark ./dedup -benchmem`

## Results

| Benchmark | ns/op | MB/s | B/op | allocs/op |
|-----------|------:|-----:|-----:|----------:|
| Fixed patterned | 46.98 | 22321696.92 | 0 | 0 |
| Fixed random | 46.92 | 22348958.15 | 0 | 0 |
| CDC patterned | 141997362 | 7.38 | 2726 | 64 |
| CDC random | 350926712 | 2.99 | 7205 | 107 |
| Hybrid patterned | 145286614 | 7.22 | 1403394 | 225 |
| Hybrid random | 341517485 | 3.07 | 1459349 | 337 |

## Bloom Filter False Positive Rates

The Bloom filter is configured with 100k inserted digests. Random digests are
tested for membership without insertion to measure the observed false positive
rate.

- Command: `go test -run '^$' -bench BloomFalsePositiveRate -benchtime=100000x -benchmem ./dedup`

| Configured FP | Observed FP |
|--------------:|------------:|
| 0.1 | 0.1027 |
| 0.01 | 0.01001 |
| 0.001 | 0.00093 |

To reduce false positives, lower the configured rate at the cost of additional
memory. `MaxChunks` estimates the number of unique chunks supported for a given
false positive rate and available RAM.

