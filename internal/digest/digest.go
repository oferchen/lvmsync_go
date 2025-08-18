package digest

import cpufeatures "lvmsync_go/internal/cpufeatures"

const (
	// SHA256 represents the SHA-256 digest algorithm.
	SHA256 = "sha256"
	// BLAKE3 represents the BLAKE3 digest algorithm.
	BLAKE3 = "blake3"
)

var (
	// HasAVX2 reports AVX2 support. Tests may override this variable.
	HasAVX2 = cpufeatures.HasAVX2
	// HasAVX512 reports AVX-512 support. Tests may override this variable.
	HasAVX512 = cpufeatures.HasAVX512
	// HasNEON reports NEON/ASIMD support. Tests may override this variable.
	HasNEON = cpufeatures.HasNEON
	// HasAESNI reports AES-NI support. Tests may override this variable.
	HasAESNI = cpufeatures.HasAESNI
)

func detect() string {
	if HasAVX512() || HasAVX2() || HasNEON() || HasAESNI() {
		return BLAKE3
	}
	return SHA256
}

// Select returns the preferred digest algorithm based on CPU capabilities.
// It chooses BLAKE3 when AVX2, AVX-512, NEON, or AES-NI are available,
// falling back to SHA-256 otherwise.
func Select() string {
	return detect()
}
