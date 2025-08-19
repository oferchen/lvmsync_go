#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
SRC_LOOP=""
DST_LOOP=""
cleanup() {
  set +e
  lvremove -f vgsrc/snap vgsrc/origin vgd/dest >/dev/null 2>&1
  vgremove -f vgsrc vgd >/dev/null 2>&1
  if [ -n "$SRC_LOOP" ]; then
    pvremove -ff "$SRC_LOOP" >/dev/null 2>&1
    losetup -d "$SRC_LOOP" >/dev/null 2>&1
  fi
  if [ -n "$DST_LOOP" ]; then
    pvremove -ff "$DST_LOOP" >/dev/null 2>&1
    losetup -d "$DST_LOOP" >/dev/null 2>&1
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

BIN="$TMPDIR/lvmsync"
go build -o "$BIN" .

# Prepare source LV
SRC_IMG="$TMPDIR/src.img"
dd if=/dev/zero of="$SRC_IMG" bs=1M count=64
SRC_LOOP=$(losetup --find --show "$SRC_IMG")
pvcreate -ffy "$SRC_LOOP"
vgcreate vgsrc "$SRC_LOOP"
lvcreate -n origin -L 32M vgsrc
# zero-filled to ensure compression
dd if=/dev/zero of=/dev/vgsrc/origin bs=1M count=32
lvcreate -s -n snap -L 32M vgsrc/origin

# Prepare destination LV
DST_IMG="$TMPDIR/dst.img"
dd if=/dev/zero of="$DST_IMG" bs=1M count=64
DST_LOOP=$(losetup --find --show "$DST_IMG")
pvcreate -ffy "$DST_LOOP"
vgcreate vgd "$DST_LOOP"
lvcreate -n dest -L 32M vgd

STATE="$TMPDIR/state.json"
"$BIN" run --compress auto --speed=1M --output=json --resume="$STATE" /dev/vgsrc/snap /dev/vgd/dest >"$TMPDIR/first.log" 2>&1 &
PID=$!
sleep 1
kill -9 $PID || true
if [ ! -f "$STATE" ]; then
  echo "resume state missing"
  exit 1
fi
if [ ! -f "$STATE.wal" ]; then
  echo "wal missing"
  exit 1
fi
BYTES1=$(grep '"bytes_transferred"' "$TMPDIR/first.log" | tail -1 | sed -E 's/.*"bytes_transferred":([0-9]+).*/\1/')

# Truncate WAL mid-frame
SIZE=$(stat -c%s "$STATE.wal")
truncate -s $((SIZE-8)) "$STATE.wal"

# Resume with verify, expect mismatch
if LVMSYNC_RESUME="$STATE" "$BIN" run --compress auto --speed=1M --output=json --resume=verify /dev/vgsrc/snap /dev/vgd/dest >"$TMPDIR/verify.log" 2>&1; then
  echo "expected resume verify to fail"
  exit 1
fi
if ! grep -q wal_verify_mismatch "$TMPDIR/verify.log"; then
  echo "missing wal_verify_mismatch"
  exit 1
fi

"$BIN" run --compress auto --speed=1M --output=json --resume="$STATE" /dev/vgsrc/snap /dev/vgd/dest >"$TMPDIR/resume.log" 2>&1
BYTES2=$(grep '"bytes_transferred"' "$TMPDIR/resume.log" | tail -1 | sed -E 's/.*"bytes_transferred":([0-9]+).*/\1/')
TOTAL=$(grep '"bytes_total"' "$TMPDIR/resume.log" | tail -1 | sed -E 's/.*"bytes_total":([0-9]+).*/\1/')
if [ $((BYTES1 + BYTES2)) -ne "$TOTAL" ]; then
  echo "resume transfer mismatch"
  exit 1
fi
if [ "$BYTES2" -ge "$TOTAL" ]; then
  echo "resume re-sent data"
  exit 1
fi

"$BIN" verify /dev/vgsrc/snap /dev/vgd/dest
exit 0
