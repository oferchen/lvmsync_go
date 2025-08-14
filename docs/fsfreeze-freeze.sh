#!/bin/sh
# Freeze the filesystem at the given mount point
# Usage: ./fsfreeze-freeze.sh /mnt/data
set -e
mount_point="$1"
fsfreeze -f "$mount_point"

