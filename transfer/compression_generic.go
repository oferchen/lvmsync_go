//go:build !amd64 && !386 && !arm64

package transfer

func detectOptimalCompression() string {
	return compressionLZ4
}
