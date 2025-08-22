package device

import "errors"

// ErrPartitionMismatch indicates the partition table does not match the expected signature or layout.
var ErrPartitionMismatch = errors.New("partition table mismatch")
