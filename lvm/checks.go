package lvm

import (
	"context"
	"errors"
	"os"
)

var (
	volumeExistsImpl = func(_ context.Context, path string) (bool, error) {
		_, err := os.Stat(path)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	autoExtendEnabledImpl = func(_ context.Context, _ string) (bool, error) { return false, nil }
	discardEnabledImpl    = func(_ context.Context, _ string) (bool, error) { return true, nil }
	isMountedImpl         = func(_ context.Context, _ string) (bool, error) { return false, nil }
)

func VolumeExists(ctx context.Context, path string) (bool, error) {
	return volumeExistsImpl(ctx, path)
}

func AutoExtendEnabled(ctx context.Context, path string) (bool, error) {
	return autoExtendEnabledImpl(ctx, path)
}

func DiscardEnabled(ctx context.Context, path string) (bool, error) {
	return discardEnabledImpl(ctx, path)
}

func IsMounted(ctx context.Context, path string) (bool, error) {
	return isMountedImpl(ctx, path)
}
