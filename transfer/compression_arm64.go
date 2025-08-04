//go:build arm64

package transfer

import "golang.org/x/sys/cpu"

func detectOptimalCompression() string {
	if cpu.ARM64.HasASIMD {
		return compressionZSTD
	}
	return compressionLZ4
}
