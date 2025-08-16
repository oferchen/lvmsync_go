package common

import "fmt"

// ValidateHandshake ensures peer and local handshakes agree on critical parameters.
func ValidateHandshake(local, peer Handshake) error {
	if local.CDCMin > 0 && local.CDCAvg > 0 && local.CDCMax > 0 {
		if !(local.CDCMin <= local.CDCAvg && local.CDCAvg <= local.CDCMax) {
			return fmt.Errorf("invalid local cdc ordering: min %d avg %d max %d", local.CDCMin, local.CDCAvg, local.CDCMax)
		}
	}
	if peer.CDCMin > 0 && peer.CDCAvg > 0 && peer.CDCMax > 0 {
		if !(peer.CDCMin <= peer.CDCAvg && peer.CDCAvg <= peer.CDCMax) {
			return fmt.Errorf("invalid peer cdc ordering: min %d avg %d max %d", peer.CDCMin, peer.CDCAvg, peer.CDCMax)
		}
	}
	if peer.Endianness != "" && local.Endianness != "" && peer.Endianness != local.Endianness {
		return fmt.Errorf("endianness mismatch: %s", peer.Endianness)
	}
	if peer.BlockSize > 0 && local.BlockSize > 0 && peer.BlockSize != local.BlockSize {
		return fmt.Errorf("block size mismatch: %d", peer.BlockSize)
	}
	if peer.DedupMode != "" && local.DedupMode != "" && peer.DedupMode != local.DedupMode {
		return fmt.Errorf("dedup mode mismatch: %s", peer.DedupMode)
	}
	if peer.CDCMin > 0 && local.CDCMin > 0 && peer.CDCMin != local.CDCMin {
		return fmt.Errorf("cdc min mismatch: %d", peer.CDCMin)
	}
	if peer.CDCAvg > 0 && local.CDCAvg > 0 && peer.CDCAvg != local.CDCAvg {
		return fmt.Errorf("cdc avg mismatch: %d", peer.CDCAvg)
	}
	if peer.CDCMax > 0 && local.CDCMax > 0 && peer.CDCMax != local.CDCMax {
		return fmt.Errorf("cdc max mismatch: %d", peer.CDCMax)
	}
	if peer.Compress != "" && local.Compress != "" && peer.Compress != local.Compress {
		return fmt.Errorf("compression mismatch: %s", peer.Compress)
	}
	if peer.CompressLevel != 0 && local.CompressLevel != 0 && peer.CompressLevel != local.CompressLevel {
		return fmt.Errorf("compression level mismatch: %d", peer.CompressLevel)
	}
	if peer.Digest != "" && local.Digest != "" && peer.Digest != local.Digest {
		return fmt.Errorf("digest mismatch: %s", peer.Digest)
	}
	if peer.CRC32C != local.CRC32C {
		return fmt.Errorf("crc32c support mismatch: %v", peer.CRC32C)
	}
	if peer.ODirect != local.ODirect {
		return fmt.Errorf("o_direct mismatch: %v", peer.ODirect)
	}
	if peer.ResumeToken != "" && local.ResumeToken != "" && peer.ResumeToken != local.ResumeToken {
		return fmt.Errorf("resume token mismatch: %s", peer.ResumeToken)
	}
	if peer.MaxInFlight > 0 && local.MaxInFlight > 0 && peer.MaxInFlight != local.MaxInFlight {
		return fmt.Errorf("max in-flight mismatch: %d", peer.MaxInFlight)
	}
	if peer.Transport != "" && local.Transport != "" && peer.Transport != local.Transport {
		return fmt.Errorf("transport mismatch: %s", peer.Transport)
	}
	if peer.ALPN != "" && local.ALPN != "" && peer.ALPN != local.ALPN {
		return fmt.Errorf("alpn mismatch: %s", peer.ALPN)
	}
	if peer.TLSVersion != "" && local.TLSVersion != "" && peer.TLSVersion != local.TLSVersion {
		return fmt.Errorf("tls version mismatch: %s", peer.TLSVersion)
	}
	return nil
}
