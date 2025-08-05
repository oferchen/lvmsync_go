package lvm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strconv"
	"strings"

	"github.com/dpeckett/lvm2"
)

type lvmBackend interface {
	CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error
	RemoveSnapshot(ctx context.Context, snapshotPath string) error
	GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error)
	GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error)
	ListVolumeGroups(ctx context.Context, candidates []string) ([]VolumeGroup, error)
}

type lvm2Backend struct {
	client *lvm2.Client
}

// EscalatedRunner implements lvm2.Runner by prepending an escalation
// command (e.g., sudo) before invoking the real LVM binary.
type EscalatedRunner struct {
	Command string
	Args    []string
}

// Run executes the LVM command via the configured escalation command.
func (r EscalatedRunner) Run(ctx context.Context, lvmPath string, args ...string) ([]byte, error) {
	all := append(append([]string{}, r.Args...), lvmPath)
	all = append(all, args...)
	cmd := exec.CommandContext(ctx, r.Command, all...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, errOut.String())
	}
	return out.Bytes(), nil
}

func newLVMBackend() lvmBackend {
	escCmd := GetEscalationCommand()
	if escCmd == "" {
		return &lvm2Backend{client: lvm2.NewClient(lvm2.WithLVM("lvm"))}
	}

	parts := strings.Fields(escCmd)
	if len(parts) == 0 {
		return &lvm2Backend{client: lvm2.NewClient(lvm2.WithLVM("lvm"))}
	}

	runner := EscalatedRunner{Command: parts[0], Args: parts[1:]}
	return &lvm2Backend{client: lvm2.NewClient(lvm2.WithLVM("lvm"), lvm2.WithRunner(runner))}
}

func (b *lvm2Backend) CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error {
	trimmed := strings.TrimPrefix(lvPath, "/dev/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid LV path: %s", lvPath)
	}
	vgName, lvName := parts[0], parts[1]

	opts := lvm2.CreateLVOptions{
		Name:     snapshotName,
		VGName:   vgName,
		Size:     size,
		Snapshot: true,
	}

	val := reflect.ValueOf(&opts).Elem()
	field := val.FieldByName("LVName")
	if !field.IsValid() {
		return fmt.Errorf("lvm2.CreateLVOptions missing LVName field")
	}
	if field.CanSet() {
		field.SetString(lvName)
	}

	return b.client.CreateLogicalVolume(ctx, opts)
}

func (b *lvm2Backend) RemoveSnapshot(ctx context.Context, snapshotPath string) error {
	opts := lvm2.RemoveLVOptions{
		Name:  snapshotPath,
		Force: true,
	}
	return b.client.RemoveLogicalVolume(ctx, opts)
}

func (b *lvm2Backend) GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error) {
	lvs, err := b.client.ListLogicalVolumes(ctx, &lvm2.ListLVOptions{
		Names: []string{snapshotPath},
	})
	if err != nil {
		return 0, err
	}
	if len(lvs) == 0 {
		return 0, fmt.Errorf("snapshot %s not found", snapshotPath)
	}
	usageStr := strings.TrimSpace(lvs[0].DataPercent)
	usage, err := strconv.ParseFloat(usageStr, 64)
	if err != nil {
		return 0, err
	}
	return usage, nil
}

func (b *lvm2Backend) GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error) {
	vgs, err := b.client.ListVolumeGroups(ctx, &lvm2.ListVGOptions{
		Names: []string{vgName},
	})
	if err != nil {
		return 0, err
	}
	if len(vgs) == 0 {
		return 0, fmt.Errorf("volume group %s not found", vgName)
	}
	freeStr := strings.TrimSpace(vgs[0].Free)
	freeStr = strings.TrimSuffix(freeStr, "B")
	free, err := strconv.ParseUint(freeStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return free, nil
}

func (b *lvm2Backend) ListVolumeGroups(ctx context.Context, candidates []string) ([]VolumeGroup, error) {
	var opts *lvm2.ListVGOptions
	if len(candidates) > 0 {
		opts = &lvm2.ListVGOptions{Names: candidates}
	}
	vgs, err := b.client.ListVolumeGroups(ctx, opts)
	if err != nil {
		return nil, err
	}
	res := make([]VolumeGroup, 0, len(vgs))
	for _, vg := range vgs {
		freeStr := strings.TrimSpace(vg.Free)
		freeStr = strings.TrimSuffix(freeStr, "B")
		free, err := strconv.ParseUint(freeStr, 10, 64)
		if err != nil {
			return nil, err
		}
		res = append(res, VolumeGroup{Name: vg.Name, Free: free})
	}
	return res, nil
}
