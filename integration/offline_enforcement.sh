#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
cleanup() {
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

BIN="$TMPDIR/lvmsync"
go build -o "$BIN" .

run_fs() (
  set -euo pipefail
  FS="$1"
  IMG="$TMPDIR/src-${FS}.img"
  MNT="$TMPDIR/mnt-${FS}"
  LOOP=""
  cleanup_fs() {
    set +e
    if mountpoint -q "$MNT"; then
      umount "$MNT" >/dev/null 2>&1
    fi
    if [ -n "$LOOP" ]; then
      losetup -d "$LOOP" >/dev/null 2>&1
    fi
    rm -f "$IMG"
    rmdir "$MNT" 2>/dev/null || true
  }
  trap cleanup_fs EXIT

  dd if=/dev/zero of="$IMG" bs=1M count=16
  LOOP=$(losetup --find --show "$IMG")
  case "$FS" in
    ext4)
      mkfs.ext4 -q "$LOOP"
      ;;
    xfs)
      mkfs.xfs -f -q "$LOOP"
      ;;
    *)
      echo "unsupported fs: $FS" >&2
      exit 1
      ;;
  esac

  mkdir "$MNT"
  mount "$LOOP" "$MNT"
  DEST="$TMPDIR/dest-${FS}.img"
  truncate -s 16M "$DEST"

  # Scenario A: no freeze/thaw commands, expect ErrPrecondition exit code
  set +e
  "$BIN" --dry-run --transport=rsync "$LOOP" "$DEST" >/tmp/offline_${FS}_a.log 2>&1
  STATUS=$?
  set -e
  if [ "$STATUS" -ne 80 ]; then
    echo "expected exit code 80, got $STATUS for $FS"
    cat /tmp/offline_${FS}_a.log
    exit 1
  fi

  # Scenario B: provide freeze and thaw commands, expect success
  FREEZE_CMD="$(pwd)/docs/fsfreeze-freeze.sh $MNT"
  THAW_CMD="$(pwd)/docs/fsfreeze-thaw.sh $MNT"
  "$BIN" --dry-run --transport=rsync --force --allow-overwrite --yes-i-know \
    --fs-freeze-command "$FREEZE_CMD" \
    --fs-thaw-command "$THAW_CMD" \
    "$LOOP" "$DEST"
)

run_fs ext4
run_fs xfs

exit 0
