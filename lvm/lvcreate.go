package lvm

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// CreateLogicalVolume creates a logical volume of the specified size in bytes.
func CreateLogicalVolume(ctx context.Context, vgName, lvName string, sizeBytes uint64, logger *zap.Logger) error {
	return defaultRunner.CreateLogicalVolume(ctx, vgName, lvName, sizeBytes, logger)
}

// CreateLogicalVolume creates a logical volume using the provided Runner.
func (r *Runner) CreateLogicalVolume(ctx context.Context, vgName, lvName string, sizeBytes uint64, logger *zap.Logger) error {
	if err := r.checkPrivs(); err != nil {
		return err
	}
	if vgName == "" || lvName == "" || sizeBytes == 0 {
		return fmt.Errorf("invalid parameters")
	}
	if err := r.backend.CreateLogicalVolume(ctx, vgName, lvName, sizeBytes); err != nil {
		return fmt.Errorf("failed to create logical volume %s/%s: %w", vgName, lvName, err)
	}
	logger.Info("logical volume created", zap.String("volume_group", vgName), zap.String("logical_volume", lvName), zap.Uint64("size_bytes", sizeBytes))
	return nil
}
