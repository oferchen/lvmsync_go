#!/bin/sh
set -euo pipefail
SRC=${1:?"source device required"}
DST=${2:?"destination path or remote target required"}
TRANSPORTS="quic h2 tcp+tls ssh"
COMPRESSIONS="none lz4 zstd"
# Example WAN emulation: 50ms RTT, 100Mbps
IFACE=${IFACE:-eth0}
# tc qdisc add dev "$IFACE" root handle 1: netem delay 25ms
# tc qdisc add dev "$IFACE" parent 1:1 handle 10: tbf rate 100mbit burst 32kbit latency 400ms
for t in $TRANSPORTS; do
  for c in $COMPRESSIONS; do
    echo "== $t/$c =="
    /usr/bin/time -f "%e sec %P cpu" lvmsync run --transport "$t" --compress "$c" --mode throughput "$SRC" "$DST"
  done
done
# tc qdisc del dev "$IFACE" root || true
