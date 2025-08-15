package transfer

import "math"

func calculateTotalDataSize(ranges []Range) int64 {
	var total uint64
	for _, r := range ranges {
		if r.End < r.Start {
			continue
		}
		total += r.End - r.Start + 1
	}
	if total > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(total)
}
