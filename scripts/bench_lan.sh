#!/bin/sh
set -euo pipefail
SRC=${1:?"source device required"}
DST=${2:?"destination path required"}
TRANSPORTS="quic h2 tcp+tls ssh"
COMPRESSIONS="none lz4 zstd"
for t in $TRANSPORTS; do
  for c in $COMPRESSIONS; do
    echo "== $t/$c =="
    /usr/bin/time -f "%e sec %P cpu" lvmsync run --transport "$t" --compress "$c" --mode throughput "$SRC" "$DST"
  done
done
