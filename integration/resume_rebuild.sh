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

STATE="$TMPDIR/state.json"
MAN="$TMPDIR/manifest"
"$BIN" run --speed=1M --output=json --manifest-path "$MAN" --resume="$STATE" /dev/vgsrc/snap /dev/vgd/dest >"$TMPDIR/first.log" 2>&1 &
PID=$!
sleep 1
kill -9 $PID || true
if [ ! -f "$STATE" ]; then
  echo "resume state missing"
  exit 1
fi
# Corrupt the manifest
truncate -s 10 "$MAN"
"$BIN" manifest rebuild --manifest-path "$MAN" /dev/vgsrc/snap

"$BIN" run --speed=1M --output=json --manifest-path "$MAN" --resume="$STATE" /dev/vgsrc/snap /dev/vgd/dest >"$TMPDIR/resume.log" 2>&1
"$BIN" verify --manifest-path "$MAN" /dev/vgsrc/snap /dev/vgd/dest
exit 0
