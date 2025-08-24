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

# Prepare destination LV
DST_IMG="$TMPDIR/dst.img"
dd if=/dev/zero of="$DST_IMG" bs=1M count=64
DST_LOOP=$(losetup --find --show "$DST_IMG")
pvcreate -ffy "$DST_LOOP"
vgcreate vgd "$DST_LOOP"
lvcreate -n dest -L 32M vgd

# Record initial destination hash
HASH_BEFORE=$(sha256sum /dev/vgd/dest | awk '{print $1}')

# Probe-only should succeed and not modify destination
"$BIN" run --probe-only /dev/vgsrc/snap /dev/vgd/dest
HASH_AFTER_PROBE=$(sha256sum /dev/vgd/dest | awk '{print $1}')
if [ "$HASH_AFTER_PROBE" != "$HASH_BEFORE" ]; then
  echo "probe-only modified destination"
  exit 1
fi

# Verify-only should exit 3 and not modify destination
set +e
"$BIN" run --verify-only /dev/vgsrc/snap /dev/vgd/dest
STATUS=$?
set -e
if [ "$STATUS" -ne 3 ]; then
  echo "expected exit code 3, got $STATUS"
  exit 1
fi
HASH_AFTER_VERIFY=$(sha256sum /dev/vgd/dest | awk '{print $1}')
if [ "$HASH_AFTER_VERIFY" != "$HASH_BEFORE" ]; then
  echo "verify-only modified destination"
  exit 1
fi

# Real run should succeed and change destination
"$BIN" run /dev/vgsrc/snap /dev/vgd/dest
HASH_AFTER_RUN=$(sha256sum /dev/vgd/dest | awk '{print $1}')
if [ "$HASH_AFTER_RUN" = "$HASH_BEFORE" ]; then
  echo "run did not modify destination"
  exit 1
fi

exit 0
