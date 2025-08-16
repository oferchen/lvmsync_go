package transfer

// Strategy constants used for compression and deduplication.
const (
	// StrategyAuto selects an optimal strategy automatically.
	StrategyAuto = "auto"
	// StrategyChecksum uses checksum-based deduplication.
	StrategyChecksum = "checksum"
	// StrategyRollingHash uses a rolling hash for deduplication.
	StrategyRollingHash = "rolling_hash"

	// intraCacheCapacity limits the number of chunks tracked for intra-run
	// deduplication.
	intraCacheCapacity = 4096
)
