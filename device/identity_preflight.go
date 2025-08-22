package device

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"lvmsync_go/internal/exitcode"
	"lvmsync_go/internal/privilege"
)

// VerifyIdentity checks that two device paths have matching size, UUID, and
// manifest epoch. Mismatches are allowed only when both --allow-overwrite and
// --yes-i-know are set in the context.
func VerifyIdentity(ctx context.Context, info *Info, src, dest string) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	allow := allowOverwriteFromContext(ctx) && yesIKnowFromContext(ctx)
	sizeA, err := info.SizeBytes(ctx, src)
	if err != nil {
		return err
	}
	sizeB, err := info.SizeBytes(ctx, dest)
	if err != nil {
		return err
	}
	if sizeA != sizeB && !allow {
		return fmt.Errorf("size mismatch: %w", exitcode.ErrPrecondition)
	}
	match, err := info.IDsMatch(ctx, src, dest)
	if err != nil {
		return err
	}
	if !match && !allow {
		return fmt.Errorf("uuid mismatch: %w", exitcode.ErrPrecondition)
	}
	devA, err := info.detectFunc(ctx, src, true, "", "", "", "", 0, 0, privilege.New(ctx, zap.NewNop()), zap.NewNop(), NewRunner())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := devA.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close block device: %w", closeErr)
		}
	}()
	devB, err := info.detectFunc(ctx, dest, true, "", "", "", "", 0, 0, privilege.New(ctx, zap.NewNop()), zap.NewNop(), NewRunner())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := devB.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close block device: %w", closeErr)
		}
	}()
	idA, err := devA.Identity(ctx)
	if err != nil {
		return err
	}
	idB, err := devB.Identity(ctx)
	if err != nil {
		return err
	}
	if idA.GPTUUID != idB.GPTUUID && !allow {
		return fmt.Errorf("gpt uuid mismatch: %w", exitcode.ErrPrecondition)
	}
	if idA.MBRSignature != idB.MBRSignature && !allow {
		return fmt.Errorf("mbr signature mismatch: %w", exitcode.ErrPrecondition)
	}
	if idA.ManifestEpoch != idB.ManifestEpoch && !allow {
		return fmt.Errorf("manifest epoch mismatch: %w", exitcode.ErrPrecondition)
	}
	return nil
}
