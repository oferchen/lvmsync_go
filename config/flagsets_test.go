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
		{"mode", cfg.Mode},
		{"parallel", strconv.Itoa(cfg.Parallel)},
		{"zerocopy", strconv.FormatBool(cfg.ZeroCopy)},
		{"odirect", strconv.FormatBool(cfg.ODirect)},
		{"numa_pin", strconv.FormatBool(cfg.NumaPin)},
		{"max_retries", strconv.Itoa(cfg.MaxRetries)},
		{"resume", cfg.ResumeState},
		{"speed", cfg.Speed},
		{"sync_interval", cfg.SyncInterval},
		{"checkpoint_interval", cfg.CheckpointInterval.String()},
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
		{"ssh_host", cfg.SSHHost},
		{"ssh_user", cfg.SSHUser},
		{"ssh_key", cfg.SSHKeyPath},
		{"ssh_port", strconv.Itoa(cfg.SSHPort)},
		{"ssh_timeout", cfg.SSHTimeout.String()},
		{"ssh_keepalive", cfg.SSHKeepAliveInterval.String()},
		{"known_hosts", cfg.KnownHosts},
		{"strict_host_key_checking", strconv.FormatBool(cfg.StrictHostKeyCheck)},
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
		{"dedup", cfg.DedupMode},
		{"cdc_min", strconv.Itoa(cfg.CDCMin)},
		{"cdc_avg", strconv.Itoa(cfg.CDCAvg)},
		{"cdc_max", strconv.Itoa(cfg.CDCMax)},
		{"dedup_strategy", cfg.DedupStrategy},
		{"dedup_state_file", cfg.DedupStateFile},
		{"bloom_entries", strconv.Itoa(cfg.BloomEntries)},
		{"bloom_fp_rate", fmt.Sprint(cfg.BloomFpRate)},
		{"bloom_mbits", strconv.FormatUint(uint64(cfg.BloomMBits), 10)},
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
		{"zstd_level", strconv.Itoa(cfg.ZstdLevel)},
		{"lz4_level", cfg.LZ4Level},
		{"compress_concurrency", strconv.Itoa(cfg.CompressConcurrency)},
		{"compress_threshold", strconv.FormatFloat(cfg.CompressThreshold, 'f', -1, 64)},
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
		{"lvm_timeout", cfg.LVMTimeout.String()},
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
		{"grpc_listen", cfg.GRPCListen},
		{"grpc_connect", cfg.GRPCConnect},
		{"grpc_dial_timeout", cfg.GRPCDialTimeout.String()},
		{"grpc_setup_timeout", cfg.GRPCSetupTimeout.String()},
		{"grpc_heartbeat_interval", cfg.HeartbeatInterval.String()},
		{"grpc_heartbeat_send_timeout", cfg.HeartbeatSendTimeout.String()},
		{"tls_cert", cfg.TLSCert},
		{"tls_key", cfg.TLSKey},
		{"ca_cert", cfg.CACert},
		{"allow_insecure", strconv.FormatBool(cfg.AllowInsecure)},
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

func TestInitTransportFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initTransportFlags(cfg)
	cases := []struct{ name, want string }{
		{"transport", cfg.Transport},
		{"concurrency", strconv.Itoa(cfg.Concurrency)},
		{"tcp_port", strconv.Itoa(cfg.TCPPort)},
		{"tcp_parallel", strconv.Itoa(cfg.TCPParallel)},
		{"tcp_lowat", strconv.Itoa(cfg.TCPNotSentLowAt)},
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
