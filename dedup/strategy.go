package dedup

// defaultFpRate is the Bloom filter false positive rate used when evaluating
// automatic strategy selection.
const defaultFpRate = 0.001

const minChunk = 4 * 1024

// AutoStrategy selects a deduplication strategy based on volume size, RAM budget,
// and whether the CPU has checksum acceleration. When a Bloom filter sized for
// the volume fits within the RAM budget, it returns "bloom". Otherwise it falls
// back to "checksum" if acceleration is available or "rolling_hash".
func AutoStrategy(volumeSize, ramBytes uint64, hasAccel bool) string {
	maxChunks, err := MaxChunks(ramBytes, defaultFpRate)
	if err == nil {
		volumeChunks := volumeSize / uint64(minChunk)
		if volumeChunks == 0 {
			volumeChunks = 1
		}
		if maxChunks >= volumeChunks {
			return "bloom"
		}
	}
	if hasAccel {
		return "checksum"
	}
	return "rolling_hash"
}

// AutoStrategyTable returns the decision table used by AutoStrategy in
// Markdown format so documentation can be kept in sync.
func AutoStrategyTable() string {
	return "| Condition | Selected strategy |\n" +
		"|-----------|------------------|\n" +
		"| Bloom filter fits in RAM | `bloom` |\n" +
		"| Doesn't fit, checksum acceleration available | `checksum` |\n" +
		"| Doesn't fit, no acceleration | `rolling_hash` |\n"
}
