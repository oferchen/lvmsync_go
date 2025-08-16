package digest

import cpufeatures "lvmsync_go/internal/cpufeatures"

const (
	// SHA256 represents the SHA-256 digest algorithm.
	SHA256 = "sha256"
	// BLAKE3 represents the BLAKE3 digest algorithm.
	BLAKE3 = "blake3"
)

var (
	hasAVX2   = cpufeatures.HasAVX2
	hasAVX512 = cpufeatures.HasAVX512
	hasNEON   = cpufeatures.HasNEON
)

func detect() string {
	if hasAVX512() || hasAVX2() || hasNEON() {
		return BLAKE3
	}
	return SHA256
}

// Select returns the preferred digest algorithm based on CPU capabilities.
// It chooses BLAKE3 when AVX2, AVX-512, or NEON are available, falling back
// to SHA-256 otherwise. The function variable allows tests to override the
// detection logic.
var Select = detect
