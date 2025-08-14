#!/bin/sh
# Thaw the filesystem at the given mount point
# Usage: ./fsfreeze-thaw.sh /mnt/data
set -e
mount_point="$1"
fsfreeze -u "$mount_point"

