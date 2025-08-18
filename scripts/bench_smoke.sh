#!/usr/bin/env bash
#
# bench_smoke.sh runs a tiny transfer with compression disabled and enabled.
# It records the throughput of each run along with the current commit hash so
# results can be compared across revisions.

set -euo pipefail

# Size of the temporary dataset in megabytes.
SIZE_MB=${SIZE_MB:-8}
BYTES=$((SIZE_MB * 1024 * 1024))
# Minimal throughput in MB/s expected for the smoke test to pass.
MIN_MBPS=${MIN_MBPS:-1}

COMMIT=$(git rev-parse --short HEAD)
OUT_DIR=$(dirname "$0")/..
OUT_FILE="$OUT_DIR/reports/bench_smoke.csv"
mkdir -p "$(dirname "$OUT_FILE")"
if [ ! -f "$OUT_FILE" ]; then
    echo "commit,compression,throughput_MBps" >"$OUT_FILE"
fi

SRC=$(mktemp)
trap 'rm -f "$SRC" "$DST"' EXIT INT TERM
dd if=/dev/urandom of="$SRC" bs=1M count="$SIZE_MB" status=none

for COMP in none zstd; do
    DST=$(mktemp)
    start=$(date +%s%N)
    ./lvmsync run --mode throughput --compress "$COMP" "$SRC" "$DST" >/dev/null 2>&1
    end=$(date +%s%N)
    rm -f "$DST"

    elapsed_ns=$((end - start))
    throughput=$(awk -v b="$BYTES" -v ns="$elapsed_ns" 'BEGIN { print (b/1048576)/(ns/1e9) }')
    printf '%s,%s,%.2f\n' "$COMMIT" "$COMP" "$throughput" >>"$OUT_FILE"
    awk -v t="$throughput" -v min="$MIN_MBPS" 'BEGIN { if (t < min) exit 1 }'
done

echo "results appended to $OUT_FILE"
