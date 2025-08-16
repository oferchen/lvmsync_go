package digest

import "testing"

func TestDetect(t *testing.T) {
	origAVX2, origAVX512, origNEON, origAESNI := hasAVX2, hasAVX512, hasNEON, hasAESNI
	t.Cleanup(func() { hasAVX2, hasAVX512, hasNEON, hasAESNI = origAVX2, origAVX512, origNEON, origAESNI })

	tests := []struct {
		name                      string
		avx2, avx512, neon, aesni bool
		want                      string
	}{
		{name: "avx2", avx2: true, want: BLAKE3},
		{name: "avx512", avx512: true, want: BLAKE3},
		{name: "neon", neon: true, want: BLAKE3},
		{name: "aesni", aesni: true, want: BLAKE3},
		{name: "none", want: SHA256},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasAVX2 = func() bool { return tt.avx2 }
			hasAVX512 = func() bool { return tt.avx512 }
			hasNEON = func() bool { return tt.neon }
			hasAESNI = func() bool { return tt.aesni }
			if got := detect(); got != tt.want {
				t.Fatalf("detect() = %s, want %s", got, tt.want)
			}
		})
	}
}
