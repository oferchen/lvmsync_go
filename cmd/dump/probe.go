package dump

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"
)

const firstBlockDigestSize = 1 << 20

func realProbeDestination(ctx context.Context, cfg *config.Config, dest string, logger *zap.Logger) (uint64, string, uint64, error) {
	info := device.NewInfo()
	id, err := info.GetDeviceID(ctx, dest)
	if err != nil {
		return 0, "", 0, fmt.Errorf("read destination id: %w", err)
	}
	size, err := info.SizeBytes(ctx, dest)
	if err != nil {
		return 0, "", 0, fmt.Errorf("read destination size: %w", err)
	}
	var epoch uint64
	if cfg.ManifestPath != "" {
		hdr, err := readManifestHeader(ctx, cfg.ManifestPath, 0)
		if err != nil {
			return 0, "", 0, err
		}
		manID := strings.TrimRight(string(hdr.DeviceID[:]), "\x00")
		if id != manID {
			return 0, "", 0, fmt.Errorf("destination device id %s does not match manifest %s", id, manID)
		}
		if size != hdr.SizeBytes {
			return 0, "", 0, fmt.Errorf("destination device size %d does not match manifest %d", size, hdr.SizeBytes)
		}
		dig, err := info.FirstBlockDigest(ctx, dest, firstBlockDigestSize)
		if err != nil {
			return 0, "", 0, fmt.Errorf("read destination digest: %w", err)
		}
		if dig != hdr.FirstBlockDigest {
			return 0, "", 0, fmt.Errorf("destination device digest mismatch")
		}
		epoch = hdr.Epoch
	}
	mounted, err := info.IsMountedRW(ctx, dest)
	if err != nil {
		return 0, "", 0, fmt.Errorf("check mount status: %w", err)
	}
	if mounted && !cfg.Force {
		logger.Error("destination_mounted_rw", zap.String("path", dest))
		return 0, "", 0, fmt.Errorf("destination device %s is mounted read-write", dest)
	}
	return size, id, epoch, nil
}

func readManifestHeader(ctx context.Context, path string, timeout time.Duration) (*manifestpkg.Header, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	f, err := common.OpenWithContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hdr manifestpkg.Header
	if err := binary.Read(f, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("read manifest header: %w", err)
	}
	if mac := manifestHeaderMAC(&hdr); !bytes.Equal(mac[:], hdr.MAC[:]) {
		return nil, fmt.Errorf("manifest: header MAC mismatch")
	}
	if hdr.Version != manifestpkg.Version {
		return nil, fmt.Errorf("manifest: version mismatch")
	}
	return &hdr, nil
}

func manifestHeaderMAC(h *manifestpkg.Header) [32]byte {
	var buf [manifestpkg.HeaderSize - 32]byte
	binary.LittleEndian.PutUint32(buf[0:4], h.Version)
	binary.LittleEndian.PutUint32(buf[4:8], h.BlockSize)
	binary.LittleEndian.PutUint64(buf[8:16], h.SizeBytes)
	binary.LittleEndian.PutUint64(buf[16:24], h.ChunkCount)
	binary.LittleEndian.PutUint32(buf[24:28], h.MinChunkSize)
	binary.LittleEndian.PutUint32(buf[28:32], h.AvgChunkSize)
	binary.LittleEndian.PutUint32(buf[32:36], h.MaxChunkSize)
	binary.LittleEndian.PutUint32(buf[36:40], h.HybridFixedSize)
	binary.LittleEndian.PutUint64(buf[40:48], h.Epoch)
	binary.LittleEndian.PutUint32(buf[48:52], h.Major)
	binary.LittleEndian.PutUint32(buf[52:56], h.Minor)
	copy(buf[56:120], h.DeviceID[:])
	copy(buf[120:152], h.FirstBlockDigest[:])
	return blake3.Sum256(buf[:])
}
