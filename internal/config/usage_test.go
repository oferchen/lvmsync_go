package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestFlagSetUsage(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	sets := []struct {
		fs   *pflag.FlagSet
		want string
	}{
		{initGeneralFlags(cfg), "--config"},
		{initSSHFlags(cfg), "--ssh_host"},
		{initRemoteFlags(cfg), "--lvmsync_path"},
		{initDedupFlags(cfg), "--dedup_strategy"},
		{initCompressionFlags(cfg), "--compress"},
		{initLVMFlags(cfg), "--skip_snapshot_creation"},
		{initGRPCFlags(cfg), "--grpc_port"},
	}
	for _, tt := range sets {
		buf := &bytes.Buffer{}
		tt.fs.SetOutput(buf)
		tt.fs.PrintDefaults()
		if !strings.Contains(buf.String(), tt.want) {
			t.Fatalf("usage for %s missing %q", tt.fs.Name(), tt.want)
		}
	}
}
