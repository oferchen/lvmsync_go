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
dd if=/dev/urandom of="$SRC_IMG" bs=1M count=64
SRC_LOOP=$(losetup --find --show "$SRC_IMG")
pvcreate -ffy "$SRC_LOOP"
vgcreate vgsrc "$SRC_LOOP"
lvcreate -n origin -L 32M vgsrc
dd if=/dev/urandom of=/dev/vgsrc/origin bs=1M count=32
lvcreate -s -n snap -L 32M vgsrc/origin

# Overwrite prevention test with smaller dest
DST_IMG_SMALL="$TMPDIR/dst_small.img"
dd if=/dev/zero of="$DST_IMG_SMALL" bs=1M count=16
DST_LOOP_SMALL=$(losetup --find --show "$DST_IMG_SMALL")
pvcreate -ffy "$DST_LOOP_SMALL"
vgcreate vgd "$DST_LOOP_SMALL"
lvcreate -n dest -L 16M vgd
if "$BIN" run /dev/vgsrc/snap /dev/vgd/dest >/tmp/overwrite.log 2>&1; then
  echo "overwrite prevention failed"
  exit 1
fi
lvremove -f vgd/dest >/dev/null 2>&1
vgremove -f vgd >/dev/null 2>&1
pvremove -ff "$DST_LOOP_SMALL" >/dev/null 2>&1
losetup -d "$DST_LOOP_SMALL" >/dev/null 2>&1

# Proper destination
DST_IMG="$TMPDIR/dst.img"
dd if=/dev/zero of="$DST_IMG" bs=1M count=64
DST_LOOP=$(losetup --find --show "$DST_IMG")
pvcreate -ffy "$DST_LOOP"
vgcreate vgd "$DST_LOOP"
lvcreate -n dest -L 32M vgd

# Mid-transfer interruptions and resume with simulated network flaps
STATE="$TMPDIR/state.json"
for delay in 1 2 3; do
  "$BIN" run --speed=1M --resume="$STATE" /dev/vgsrc/snap /dev/vgd/dest &
  PID=$!
  sleep "$delay"
  # Simulate an SSH session drop
  kill -HUP $PID || true
  sleep 1
  kill -9 $PID 2>/dev/null || true
  if [ ! -f "$STATE" ]; then
    echo "resume state missing"
    exit 1
  fi
done
"$BIN" run --resume="$STATE" /dev/vgsrc/snap /dev/vgd/dest
"$BIN" verify /dev/vgsrc/snap /dev/vgd/dest

# Dry-run estimate
"$BIN" run --dry-run /dev/vgsrc/snap /dev/vgd/dest | grep 'dry run'

# Verify-only
"$BIN" run --verify-only /dev/vgsrc/snap /dev/vgd/dest

exit 0
