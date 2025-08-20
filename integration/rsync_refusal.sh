#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
cleanup(){
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

BIN="$TMPDIR/lvmsync"
go build -o "$BIN" .

SRC="$TMPDIR/src.img"
DST_NEG="$TMPDIR/dst_neg.img"
DST_POS="$TMPDIR/dst_pos.img"

dd if=/dev/zero of="$SRC" bs=1M count=1
cp "$SRC" "$DST_NEG"
cp "$SRC" "$DST_POS"

# Scenario A: rsync transport without allow-insecure should fail
set +e
"$BIN" --transport=rsync --force --allow-overwrite --yes-i-know "$SRC" "$DST_NEG" >"$TMPDIR/neg.log" 2>&1
STATUS=$?
set -e
if [ "$STATUS" -eq 0 ]; then
  echo "expected failure without --allow-insecure"
  cat "$TMPDIR/neg.log"
  exit 1
fi
if ! grep -q "unsupported transport" "$TMPDIR/neg.log"; then
  echo "missing unsupported transport warning"
  cat "$TMPDIR/neg.log"
  exit 1
fi

# Scenario B: rsync transport with allow-insecure should succeed with plaintext warning
"$BIN" --transport=rsync --allow-insecure --force --allow-overwrite --yes-i-know "$SRC" "$DST_POS" >"$TMPDIR/pos.log" 2>&1
if ! grep -q "plaintext_connection" "$TMPDIR/pos.log"; then
  echo "missing plaintext warning"
  cat "$TMPDIR/pos.log"
  exit 1
fi

exit 0
