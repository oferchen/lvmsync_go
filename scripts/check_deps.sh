#!/usr/bin/env bash
set -euo pipefail

if ! command -v pkg-config >/dev/null 2>&1; then
  echo "pkg-config is required but not installed" >&2
  exit 1
fi

if ! pkg-config --exists devmapper; then
  echo "device-mapper development files are missing" >&2
  exit 1
fi

if pkg-config --exists lvm2; then
  exit 0
fi

if [ ! -f /usr/include/lvm2cmd.h ]; then
  echo "liblvm2 development headers (liblvm2-dev) are required" >&2
  exit 1
fi
