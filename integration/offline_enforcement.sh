#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
IMG="$TMPDIR/src.img"
MNT="$TMPDIR/mnt"
LOOP=""
cleanup() {
  set +e
  if mountpoint -q "$MNT"; then
    umount "$MNT" >/dev/null 2>&1
  fi
  if [ -n "$LOOP" ]; then
    losetup -d "$LOOP" >/dev/null 2>&1
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

BIN="$TMPDIR/lvmsync"
go build -o "$BIN" .

dd if=/dev/zero of="$IMG" bs=1M count=16
LOOP=$(losetup --find --show "$IMG")
mkfs.ext4 -q "$LOOP"
mkdir "$MNT"
mount "$LOOP" "$MNT"
DEST="$TMPDIR/dest.img"
truncate -s 16M "$DEST"

# Scenario A: no freeze/thaw commands, expect ErrPrecondition exit code
set +e
"$BIN" --dry-run --transport=rsync "$LOOP" "$DEST" >/tmp/offline_a.log 2>&1
STATUS=$?
set -e
if [ "$STATUS" -ne 80 ]; then
  echo "expected exit code 80, got $STATUS"
  cat /tmp/offline_a.log
  exit 1
fi

# Scenario B: provide freeze and thaw commands, expect success
FREEZE_CMD="$(pwd)/docs/fsfreeze-freeze.sh $MNT"
THAW_CMD="$(pwd)/docs/fsfreeze-thaw.sh $MNT"
"$BIN" --dry-run --transport=rsync --force --allow-overwrite \
  --fs-freeze-command "$FREEZE_CMD" \
  --fs-thaw-command "$THAW_CMD" \
  "$LOOP" "$DEST"

exit 0
