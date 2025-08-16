package transfer

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
)

func openApplyReader(applyFile string) (io.ReadCloser, error) {
	if applyFile == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	f, err := os.Open(applyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open apply file %s: %w", applyFile, err)
	}
	return f, nil
}

func (t *Transfer) applyData(cfg *config.Config, in io.Reader, destDevice string) error {
	if cfg.DeviceUUID != "" {
		uuid, err := device.GetDeviceID(context.Background(), destDevice)
		if err != nil {
			return fmt.Errorf("read destination uuid: %w", err)
		}
		if uuid != cfg.DeviceUUID {
			return fmt.Errorf("destination device uuid %s does not match expected %s", uuid, cfg.DeviceUUID)
		}
	}
	mounted, err := device.IsMountedRW(destDevice)
	if err != nil {
		return fmt.Errorf("check mount status: %w", err)
	}
	if mounted && !cfg.Force {
		return fmt.Errorf("destination device %s is mounted read-write", destDevice)
	}
	dedup := NewDeduplicationStrategy(cfg, t.Logger)
	if dedup != nil {
		t.Logger.Info("Applying deduplication during restore", zap.String("strategy", cfg.DedupStrategy))
		defer func() {
			if err := dedup.SaveState(); err != nil {
				t.Logger.Error("Failed to save dedup state", zap.Error(err))
			}
		}()
		return t.ProcessDumpDataWithDeduplication(context.Background(), cfg, in, destDevice, dedup)
	}
	return t.ProcessDumpData(context.Background(), cfg, in, destDevice)
}

// RunApply reads a dump file or stdin and writes the data to destDevice.
func (t *Transfer) RunApply(cfg *config.Config, applyFile, destDevice string) (err error) {
	rc, err := openApplyReader(applyFile)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(rc, &err, "close apply file")
	err = t.applyData(cfg, rc, destDevice)
	finalizeResumeState(cfg, t.Tracker, t.Logger)
	return err
}
