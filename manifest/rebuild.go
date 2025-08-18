package manifest

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
)

// Regenerate verifies an existing manifest for the device and rebuilds it when missing or stale.
// The rebuild respects the same options as Rebuild.
func Regenerate(
	ctx context.Context,
	devicePath, manifestPath string,
	logger *zap.Logger,
	interval time.Duration,
	allowMounted bool,
	cdcMin, cdcAvg, cdcMax, hybridFixed uint32,
	opts ...IndexOption,
) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	idx, err := Open(manifestPath)
	if err == nil {
		defer idx.Close()
		info, err := os.Stat(devicePath)
		if err != nil {
			return err
		}
		if uint64(info.Size()) != idx.hdr.SizeBytes || idx.hdr.BlockSize == 0 {
			return rebuild(ctx, devicePath, manifestPath, logger, interval, allowMounted, cdcMin, cdcAvg, cdcMax, hybridFixed, idx, opts...)
		}
		f, err := os.Open(devicePath)
		if err != nil {
			return err
		}
		defer f.Close()
		buf := make([]byte, idx.hdr.BlockSize)
		for i := uint64(0); i < idx.hdr.ChunkCount; i++ {
			if err = ctx.Err(); err != nil {
				return err
			}
			off, length, _, xx, digest, err := idx.Entry(i)
			if err != nil {
				return fmt.Errorf("manifest entry: %w", err)
			}
			if int(length) > cap(buf) {
				buf = make([]byte, int(length))
			}
			data := buf[:int(length)]
			if _, err := f.ReadAt(data, int64(off)); err != nil && err != io.EOF {
				return fmt.Errorf("read device: %w", err)
			}
			if xxh3.Hash(data) != xx || blake3.Sum256(data) != digest {
				return rebuild(ctx, devicePath, manifestPath, logger, interval, allowMounted, cdcMin, cdcAvg, cdcMax, hybridFixed, idx, opts...)
			}
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return Rebuild(ctx, devicePath, manifestPath, logger, interval, allowMounted, cdcMin, cdcAvg, cdcMax, hybridFixed, opts...)
}

func rebuild(
	ctx context.Context,
	devicePath, manifestPath string,
	logger *zap.Logger,
	interval time.Duration,
	allowMounted bool,
	cdcMin, cdcAvg, cdcMax, hybridFixed uint32,
	idx *Index,
	opts ...IndexOption,
) error {
	if idx != nil {
		idx.Close()
	}
	return Rebuild(ctx, devicePath, manifestPath, logger, interval, allowMounted, cdcMin, cdcAvg, cdcMax, hybridFixed, opts...)
}
