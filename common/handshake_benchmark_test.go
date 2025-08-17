package common

import (
	"strconv"
	"testing"
)

// selectBestSlice mirrors the original SelectBest implementation using nested loops.
func selectBestSlice(local, remote []string) string {
	for _, l := range local {
		for _, r := range remote {
			if l == r {
				return l
			}
		}
	}
	if len(local) > 0 {
		return local[0]
	}
	return ""
}

var (
	benchLocal  []string
	benchRemote []string
)

func init() {
	const size = 1000
	benchLocal = make([]string, size)
	benchRemote = make([]string, size)
	for i := 0; i < size; i++ {
		benchLocal[i] = strconv.Itoa(i)
		benchRemote[i] = "x" + strconv.Itoa(i)
	}
	benchRemote[size-1] = benchLocal[size-1]
}

func BenchmarkSelectBestSlice(b *testing.B) {
	for i := 0; i < b.N; i++ {
		selectBestSlice(benchLocal, benchRemote)
	}
}

func BenchmarkSelectBestMap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		SelectBest(benchLocal, benchRemote)
	}
}
