package config

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"
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
		{"dry-run", strconv.FormatBool(cfg.DryRun)},
		{"force", strconv.FormatBool(cfg.Force)},
		{"discard", strconv.FormatBool(cfg.Discard)},
		{"offline", strconv.FormatBool(cfg.Offline)},
		{"fs-freeze-command", cfg.FSFreezeCommand},
		{"fs-thaw-command", cfg.FSThawCommand},
		{"freeze-timeout", cfg.FreezeTimeout.String()},
		{"thaw-timeout", cfg.ThawTimeout.String()},
		{"mode", cfg.Mode},
		{"parallel", strconv.Itoa(cfg.Parallel)},
		{"zerocopy", strconv.FormatBool(cfg.ZeroCopy)},
		{"odirect", strconv.FormatBool(cfg.ODirect)},
		{"numa-pin", strconv.FormatBool(cfg.NumaPin)},
		{"max-retries", strconv.Itoa(cfg.MaxRetries)},
		{"resume", cfg.ResumeState},
		{"speed", cfg.Speed},
		{"sync-interval", cfg.SyncInterval},
		{"checkpoint-bytes", cfg.CheckpointBytesRaw},
		{"checkpoint-interval", cfg.CheckpointInterval.String()},
		{"block-size", cfg.BlockSizeRaw},
		{"verbose", "0"},
		{"verify-checksum", strconv.FormatBool(cfg.VerifyChecksum)},
		{"verify", cfg.VerifyLevel},
		{"digest", cfg.ChecksumAlgorithm},
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
	if f := fs.Lookup("checksum-algorithm"); f != nil {
		t.Fatalf("unexpected checksum_algorithm flag")
	}
}

func TestDigestFlagBindsToConfig(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initGeneralFlags(cfg)
	if err := fs.Parse([]string{"--digest", "sha256"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	v := viper.New()
	if err := v.BindPFlags(fs); err != nil {
		t.Fatalf("bind: %v", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.ChecksumAlgorithm != "sha256" {
		t.Fatalf("got %s want sha256", c.ChecksumAlgorithm)
	}
}

func TestInitSSHFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initSSHFlags(cfg)
	cases := []struct{ name, want string }{
		{"ssh-host", cfg.SSHHost},
		{"ssh-user", cfg.SSHUser},
		{"ssh-key", cfg.SSHKeyPath},
		{"ssh_host_key_path", cfg.SSHHostKeyPath},
		{"ssh-port", strconv.Itoa(cfg.SSHPort)},
		{"ssh-timeout", cfg.SSHTimeout.String()},
		{"ssh-keepalive", cfg.SSHKeepAliveInterval.String()},
		{"ssh_host_key", cfg.SSHHostKey},
		{"known-hosts", cfg.KnownHosts},
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
		{"lvmsync-path", cfg.LVMSyncPath},
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
		{"cdc-min", strconv.Itoa(cfg.CDCMin)},
		{"cdc-avg", strconv.Itoa(cfg.CDCAvg)},
		{"cdc-max", strconv.Itoa(cfg.CDCMax)},
		{"dedup-strategy", cfg.DedupStrategy},
		{"dedup_state_file", cfg.DedupStateFile},
		{"intra-dedup", strconv.FormatBool(cfg.IntraDedup)},
		{"bloom-entries", strconv.Itoa(cfg.BloomEntries)},
		{"bloom_fp_rate", fmt.Sprint(cfg.BloomFpRate)},
		{"bloom-mbits", strconv.FormatUint(uint64(cfg.BloomMBits), 10)},
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
		{"zstd-level", strconv.Itoa(cfg.ZstdLevel)},
		{"lz4-level", cfg.LZ4Level},
		{"compress-concurrency", strconv.Itoa(cfg.CompressConcurrency)},
		{"compress-threshold", strconv.FormatFloat(cfg.CompressThreshold, 'f', -1, 64)},
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
		{"snapshot-size", cfg.SnapshotSize},
		{"lvm-escalation", cfg.LVMEscalation},
		{"lvm-timeout", cfg.LVMTimeout.String()},
		{"sig-cache-ttl", cfg.SigCacheTTL.String()},
		{"sig-cache-max", strconv.Itoa(cfg.SigCacheMax)},
		{"volume-group", cfg.VolumeGroup},
		{"target_volume_group", cfg.TargetVolumeGroup},
		{"target-vgs", "[]"},
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
		{"grpc-port", strconv.Itoa(cfg.GRPCPort)},
		{"grpc-listen", cfg.GRPCListen},
		{"grpc-connect", cfg.GRPCConnect},
		{"grpc_dial_timeout", cfg.GRPCDialTimeout.String()},
		{"grpc_setup_timeout", cfg.GRPCSetupTimeout.String()},
		{"grpc_heartbeat_interval", cfg.HeartbeatInterval.String()},
		{"grpc_heartbeat_send_timeout", cfg.HeartbeatSendTimeout.String()},
		{"tls-cert", cfg.TLSCert},
		{"tls-key", cfg.TLSKey},
		{"ca-cert", cfg.CACert},
		{"allow-insecure", strconv.FormatBool(cfg.AllowInsecure)},
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

func TestInitManifestFlags(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initManifestFlags(cfg)
	cases := []struct{ name, want string }{
		{"manifest-path", cfg.ManifestPath},
		{"manifest-timeout", cfg.ManifestTimeout.String()},
		{"manifest_progress_interval", cfg.ManifestProgressInterval.String()},
		{"manifest_allow_mounted", strconv.FormatBool(cfg.ManifestAllowMounted)},
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
		{"tcp-port", strconv.Itoa(cfg.TCPPort)},
		{"tcp-parallel", strconv.Itoa(cfg.TCPParallel)},
		{"tcp-lowat", strconv.Itoa(cfg.TCPNotSentLowAt)},
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

func TestBindDedupEnv(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initDedupFlags(cfg)
	v := viper.New()
	if err := bindDedupEnv(fs, v); err != nil {
		t.Fatalf("bindDedupEnv: %v", err)
	}
	t.Setenv("LVMSYNC_DEDUP_STRATEGY", "checksum")
	t.Setenv("LVMSYNC_DEDUP_INTRA_DEDUP", "true")
	if got := v.GetString("dedup-strategy"); got != "checksum" {
		t.Fatalf("dedup-strategy got %q want %q", got, "checksum")
	}
	if got := v.GetBool("intra-dedup"); !got {
		t.Fatalf("intra-dedup got %v want true", got)
	}
}

func TestBindLVMEnv(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initLVMFlags(cfg)
	v := viper.New()
	if err := bindLVMEnv(fs, v); err != nil {
		t.Fatalf("bindLVMEnv: %v", err)
	}
	t.Setenv("LVMSYNC_LVM_SNAPSHOT_SIZE", "10%")
	t.Setenv("LVMSYNC_LVM_ESCALATION", "doas")
	t.Setenv("LVMSYNC_LVM_SIG_CACHE_TTL", "1m")
	if got := v.GetString("snapshot-size"); got != "10%" {
		t.Fatalf("snapshot_size got %q want %q", got, "10%")
	}
	if got := v.GetString("lvm-escalation"); got != "doas" {
		t.Fatalf("lvm-escalation got %q want %q", got, "doas")
	}
	if got := v.GetDuration("sig-cache-ttl"); got != time.Minute {
		t.Fatalf("sig-cache-ttl got %v want %v", got, time.Minute)
	}
}

func TestBindGRPCEnv(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := initGRPCFlags(cfg)
	v := viper.New()
	if err := bindGRPCEnv(fs, v); err != nil {
		t.Fatalf("bindGRPCEnv: %v", err)
	}
	t.Setenv("LVMSYNC_GRPC_PORT", "9443")
	t.Setenv("LVMSYNC_GRPC_TLS_CERT", "cert.pem")
	if got := v.GetInt("grpc-port"); got != 9443 {
		t.Fatalf("grpc_port got %d want %d", got, 9443)
	}
	if got := v.GetString("tls-cert"); got != "cert.pem" {
		t.Fatalf("tls_cert got %q want %q", got, "cert.pem")
	}
}
