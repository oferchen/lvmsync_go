package verify

import (
	"fmt"
	"io"

	"go.uber.org/zap"
	"github.com/oferchen/lvmsync_go/internal/blockio"
	cfgpkg "github.com/oferchen/lvmsync_go/internal/config"
)

// verifyInline compares two devices block-by-block and returns an error if they differ.
func verifyInline(cfg *cfgpkg.Config, src, dst string, logger *zap.Logger) error {
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = 8 * 1024 * 1024
	}
	fSrc, err := blockio.Open(src, cfg.ODirect, false)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer fSrc.Close()
	fDst, err := blockio.Open(dst, cfg.ODirect, false)
	if err != nil {
		return fmt.Errorf("open dest: %w", err)
	}
	defer fDst.Close()

	sizeSrc, err := fSrc.Size()
	if err != nil {
		return fmt.Errorf("source size: %w", err)
	}
	sizeDst, err := fDst.Size()
	if err != nil {
		return fmt.Errorf("dest size: %w", err)
	}
	if sizeSrc != sizeDst {
		logger.Error("size mismatch", zap.Int64("source_bytes", sizeSrc), zap.Int64("dest_bytes", sizeDst))
		return fmt.Errorf("size mismatch")
	}

	total := sizeSrc
	mismatches := 0
	bufSrc := make([]byte, blockSize)
	bufDst := make([]byte, blockSize)
	digest, err := digestFunc(cfg)
	if err != nil {
		return err
	}
	for off := int64(0); off < total; off += int64(blockSize) {
		size := blockSize
		if remaining := int(total - off); remaining < size {
			size = remaining
		}
		n, err := fSrc.ReadAt(bufSrc[:size], off)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read source: %w", err)
		}
		if n != size {
			return fmt.Errorf("read source: short read: expected %d, got %d", size, n)
		}
		n, err = fDst.ReadAt(bufDst[:size], off)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read dest: %w", err)
		}
		if n != size {
			return fmt.Errorf("read dest: short read: expected %d, got %d", size, n)
		}
		if digest(bufSrc[:size]) != digest(bufDst[:size]) {
			mismatches++
			logger.Error("mismatched_block", zap.Int64("offset_bytes", off))
		}
	}
	if mismatches > 0 {
		return fmt.Errorf("%d blocks differ", mismatches)
	}
	logger.Info("verification complete")
	return nil
}
