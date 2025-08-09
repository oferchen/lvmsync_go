package config

import (
	"fmt"
	"strconv"
	"testing"
)

func TestInitGeneralFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initGeneralFlags(cfg)
	cases := []struct{ name, want string }{
		{"config", ""},
		{"apply", cfg.ApplyMode},
		{"stdout", strconv.FormatBool(cfg.StdoutMode)},
		{"parallel", strconv.Itoa(cfg.Parallel)},
		{"zerocopy", strconv.FormatBool(cfg.ZeroCopy)},
		{"max_retries", strconv.Itoa(cfg.MaxRetries)},
		{"resume", cfg.ResumeState},
		{"speed", cfg.Speed},
		{"block_size", cfg.BlockSizeRaw},
		{"verbose", "0"},
		{"verify_checksum", strconv.FormatBool(cfg.VerifyChecksum)},
		{"checksum_algorithm", cfg.ChecksumAlgorithm},
		{"progress", strconv.FormatBool(cfg.Progress)},
	}
	for _, tt := range cases {
		f := fs.Lookup(tt.name)
		if f == nil {
			t.Fatalf("missing %s flag", tt.name)
		}
		if f.DefValue != tt.want {
			t.Fatalf("%s default %s want %s", tt.name, f.DefValue, tt.want)
		}
	}
}

func TestInitSSHFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initSSHFlags(cfg)
	cases := []struct{ name, want string }{
		{"ssh_user", cfg.SSHUser},
		{"ssh_key", cfg.SSHKeyPath},
		{"ssh_port", strconv.Itoa(cfg.SSHPort)},
		{"ssh_timeout", cfg.SSHTimeout.String()},
		{"ssh_keepalive", cfg.SSHKeepAliveInterval.String()},
		{"known_hosts", cfg.KnownHosts},
		{"stricthostkeychecking", strconv.FormatBool(cfg.StrictHostKeyCheck)},
	}
	for _, tt := range cases {
		f := fs.Lookup(tt.name)
		if f == nil {
			t.Fatalf("missing %s flag", tt.name)
		}
		if f.DefValue != tt.want {
			t.Fatalf("%s default %s want %s", tt.name, f.DefValue, tt.want)
		}
	}
}

func TestInitRemoteFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initRemoteFlags(cfg)
	cases := []struct{ name, want string }{
		{"lvmsync_path", cfg.LVMSyncPath},
		{"remote_pre_script", cfg.RemotePreScript},
		{"remote_post_script", cfg.RemotePostScript},
	}
	for _, tt := range cases {
		f := fs.Lookup(tt.name)
		if f == nil {
			t.Fatalf("missing %s flag", tt.name)
		}
		if f.DefValue != tt.want {
			t.Fatalf("%s default %s want %s", tt.name, f.DefValue, tt.want)
		}
	}
}

func TestInitDedupFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initDedupFlags(cfg)
	cases := []struct{ name, want string }{
		{"dedup_strategy", cfg.DedupStrategy},
		{"dedup_state_file", cfg.DedupStateFile},
		{"bloom_entries", strconv.Itoa(cfg.BloomEntries)},
		{"bloom_fp_rate", fmt.Sprint(cfg.BloomFpRate)},
	}
	for _, tt := range cases {
		f := fs.Lookup(tt.name)
		if f == nil {
			t.Fatalf("missing %s flag", tt.name)
		}
		if f.DefValue != tt.want {
			t.Fatalf("%s default %s want %s", tt.name, f.DefValue, tt.want)
		}
	}
}

func TestInitCompressionFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initCompressionFlags(cfg)
	cases := []struct{ name, want string }{
		{"compress", cfg.Compress},
		{"compress_level", strconv.Itoa(cfg.CompressLevel)},
		{"compress_concurrency", strconv.Itoa(cfg.CompressConcurrency)},
	}
	for _, tt := range cases {
		f := fs.Lookup(tt.name)
		if f == nil {
			t.Fatalf("missing %s flag", tt.name)
		}
		if f.DefValue != tt.want {
			t.Fatalf("%s default %s want %s", tt.name, f.DefValue, tt.want)
		}
	}
}

func TestInitLVMFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initLVMFlags(cfg)
	cases := []struct{ name, want string }{
		{"skip_snapshot_creation", strconv.FormatBool(cfg.SkipSnapshotCreation)},
		{"skip_disk_check", strconv.FormatBool(cfg.SkipDiskCheck)},
		{"snapshot_size", cfg.SnapshotSize},
		{"lvm_escalation", cfg.LVMEscalation},
		{"volume_group", cfg.VolumeGroup},
		{"target_volume_group", cfg.TargetVolumeGroup},
		{"target_vgs", "[]"},
	}
	for _, tt := range cases {
		f := fs.Lookup(tt.name)
		if f == nil {
			t.Fatalf("missing %s flag", tt.name)
		}
		if f.DefValue != tt.want {
			t.Fatalf("%s default %s want %s", tt.name, f.DefValue, tt.want)
		}
	}
}

func TestInitGRPCFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initGRPCFlags(cfg)
	cases := []struct{ name, want string }{
		{"grpc_port", strconv.Itoa(cfg.GRPCPort)},
		{"tls_cert", cfg.TLSCert},
		{"tls_key", cfg.TLSKey},
		{"ca_cert", cfg.CACert},
		{"allow_insecure", strconv.FormatBool(cfg.AllowInsecure)},
		{"sudo_path", cfg.SudoPath},
	}
	for _, tt := range cases {
		f := fs.Lookup(tt.name)
		if f == nil {
			t.Fatalf("missing %s flag", tt.name)
		}
		if f.DefValue != tt.want {
			t.Fatalf("%s default %s want %s", tt.name, f.DefValue, tt.want)
		}
	}
}
