//go:build amd64 || 386

package transfer

import "golang.org/x/sys/cpu"

func detectOptimalCompression() string {
	if cpu.X86.HasAVX2 {
		return compressionZSTD
	}
	return compressionLZ4
}
