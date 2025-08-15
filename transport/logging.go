package transport

import (
	"go.uber.org/zap"

	"lvmsync_go/common"
)

// HandshakeFields returns zap fields logging handshake parameters.
func HandshakeFields(h common.Handshake) []zap.Field {
	return []zap.Field{
		zap.String("dedup_mode", h.DedupMode),
		zap.Int("block_size_bytes", h.BlockSize),
		zap.String("compress", h.Compress),
		zap.Int("compress_level", h.CompressLevel),
		zap.String("digest", h.Digest),
		zap.String("resume_token", h.ResumeToken),
		zap.Bool("checksum", h.Checksum),
		zap.Bool("checksum_dedup", h.ChecksumDedup),
		zap.String("endianness", h.Endianness),
		zap.Bool("odirect", h.ODirect),
		zap.Int("max_inflight", h.MaxInFlight),
		zap.Int("cdc_min", h.CDCMin),
		zap.Int("cdc_avg", h.CDCAvg),
		zap.Int("cdc_max", h.CDCMax),
		zap.String("transport", h.Transport),
		zap.String("alpn", h.ALPN),
		zap.String("tls_version", h.TLSVersion),
	}
}
