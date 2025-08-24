#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
SRC_LOOP=""
DST_LOOP=""
cleanup() {
  set +e
  if [ -n "$SRC_LOOP" ]; then
    losetup -d "$SRC_LOOP" >/dev/null 2>&1
  fi
  if [ -n "$DST_LOOP" ]; then
    losetup -d "$DST_LOOP" >/dev/null 2>&1
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

BIN="$TMPDIR/lvmsync"
go build -o "$BIN" .

SRC_IMG="$TMPDIR/src.img"
DST_IMG="$TMPDIR/dst.img"
dd if=/dev/zero of="$SRC_IMG" bs=1M count=8
dd if=/dev/zero of="$DST_IMG" bs=1M count=8
SRC_LOOP=$(losetup --find --show "$SRC_IMG")
DST_LOOP=$(losetup --find --show "$DST_IMG")

printf ',8M,L\n' | sfdisk "$SRC_LOOP"
printf ',8M,L\n' | sfdisk "$DST_LOOP"
printf '\xDE\xAD\xBE\xEF' | dd of="$SRC_LOOP" bs=1 seek=440 conv=notrunc
printf '\xDE\xAD\xBE\xEF' | dd of="$DST_LOOP" bs=1 seek=440 conv=notrunc

"$BIN" run --offline --skip-snapshot-creation --check-partition "$SRC_LOOP" "$DST_LOOP"

printf ',4M,L\n' | sfdisk "$DST_LOOP"
printf '\xDE\xAD\xBE\xEF' | dd of="$DST_LOOP" bs=1 seek=440 conv=notrunc

"$BIN" run --offline --skip-snapshot-creation --force --verify none --check-partition "$SRC_LOOP" "$DST_LOOP"

printf ',4M,L\n' | sfdisk "$DST_LOOP"
printf '\xDE\xAD\xBE\xEF' | dd of="$DST_LOOP" bs=1 seek=440 conv=notrunc

set +e
"$BIN" run --offline --skip-snapshot-creation --force --verify inline --check-partition "$SRC_LOOP" "$DST_LOOP"
status=$?
set -e
if [ "$status" -ne 2 ]; then
  echo "expected exit code 2, got $status"
  exit 1
fi

exit 0
