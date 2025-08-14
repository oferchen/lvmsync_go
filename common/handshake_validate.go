package common

import "fmt"

// ValidateHandshake ensures peer and local handshakes agree on critical parameters.
func ValidateHandshake(local, peer Handshake) error {
	if peer.Endianness != "" && local.Endianness != "" && peer.Endianness != local.Endianness {
		return fmt.Errorf("endianness mismatch: %s", peer.Endianness)
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
	if peer.ODirect != local.ODirect {
		return fmt.Errorf("o_direct mismatch: %v", peer.ODirect)
	}
	if peer.ResumeToken != "" && local.ResumeToken != "" && peer.ResumeToken != local.ResumeToken {
		return fmt.Errorf("resume token mismatch: %s", peer.ResumeToken)
	}
	if peer.MaxInFlight > 0 && local.MaxInFlight > 0 && peer.MaxInFlight != local.MaxInFlight {
		return fmt.Errorf("max in-flight mismatch: %d", peer.MaxInFlight)
	}
	return nil
}
