package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"lvmsync_go/internal/sizeparse"
)

const (
	// Auto represents automatic detection behavior for configurable values.
	Auto = "auto"
	// Zstd represents the Zstandard compression algorithm.
	Zstd = "zstd"
)

var (
	SupportedCompression        = []string{"none", "lz4", Zstd, Auto}
	SupportedDedupStrategies    = []string{"none", Auto, "checksum", "rolling_hash", "bloom"}
	SupportedDedupModes         = []string{"fixed", "cdc", "hybrid"}
	SupportedChecksumAlgorithms = []string{"sha256", "blake3", "blake3-512", Auto}
)

type Config struct {
	SourceType               string        `mapstructure:"source-type"`
	DestType                 string        `mapstructure:"dest-type"`
	ConfigFile               string        `mapstructure:"config"`
	ApplyMode                string        `mapstructure:"apply"`
	StdoutMode               bool          `mapstructure:"stdout"`
	DryRun                   bool          `mapstructure:"dry-run"`
	Force                    bool          `mapstructure:"force"`
	Offline                  bool          `mapstructure:"offline"`
	FSFreezeCommand          string        `mapstructure:"fs-freeze-command"`
	FSThawCommand            string        `mapstructure:"fs-thaw-command"`
	FreezeTimeout            time.Duration `mapstructure:"freeze_timeout"`
	ThawTimeout              time.Duration `mapstructure:"thaw_timeout"`
	Mode                     string        `mapstructure:"mode"`
	Parallel                 int           `mapstructure:"parallel"`
	Concurrency              int           `mapstructure:"concurrency"`
	ZeroCopy                 bool          `mapstructure:"zerocopy"`
	ODirect                  bool          `mapstructure:"odirect"`
	NumaPin                  bool          `mapstructure:"numa_pin"`
	MaxRetries               int           `mapstructure:"max_retries"`
	ResumeState              string        `mapstructure:"resume"`
	ResumeToken              string        `mapstructure:"resume_token"`
	DeviceUUID               string        `mapstructure:"device_uuid"`
	SSHHost                  string        `mapstructure:"ssh_host"`
	SSHUser                  string        `mapstructure:"ssh_user"`
	SSHPassword              string        `mapstructure:"ssh_password"`
	SSHKeyPath               string        `mapstructure:"ssh_key"`
	SSHPort                  int           `mapstructure:"ssh_port"`
	SSHTimeout               time.Duration `mapstructure:"ssh_timeout"`
	SSHKeepAliveInterval     time.Duration `mapstructure:"ssh_keepalive"`
	SSHHostKey               string        `mapstructure:"ssh_host_key"`
	KnownHosts               string        `mapstructure:"known_hosts"`
	StrictHostKeyCheck       bool          `mapstructure:"strict_host_key_checking"`
	LVMSyncPath              string        `mapstructure:"lvmsync_path"`
	RemotePreScript          string        `mapstructure:"remote_pre_script"`
	RemotePostScript         string        `mapstructure:"remote_post_script"`
	Compress                 string        `mapstructure:"compress"`
	ZstdLevel                int           `mapstructure:"zstd_level"`
	LZ4Level                 string        `mapstructure:"lz4_level"`
	CompressLevel            int           `mapstructure:"-"`
	CompressConcurrency      int           `mapstructure:"compress_concurrency"`
	CompressThreshold        float64       `mapstructure:"compress_threshold"`
	Speed                    string        `mapstructure:"speed"`
	SpeedLimit               int           `mapstructure:"-"`
	VerifyChecksum           bool          `mapstructure:"verify_checksum"`
	ChecksumAlgorithm        string        `mapstructure:"checksum_algorithm"`
	Verbose                  int           `mapstructure:"verbose"`
	SkipSnapshotCreation     bool          `mapstructure:"skip_snapshot_creation"`
	SkipDiskCheck            bool          `mapstructure:"skip_disk_check"`
	SnapshotSize             string        `mapstructure:"snapshot_size"`
	VolumeGroup              string        `mapstructure:"volume_group"`
	TargetVolumeGroup        string        `mapstructure:"target_volume_group"`
	TargetVGCandidates       []string      `mapstructure:"target_vgs"`
	LVMEscalation            string        `mapstructure:"lvm_escalation"`
	LVMTimeout               time.Duration `mapstructure:"lvm_timeout"`
	Progress                 bool          `mapstructure:"progress"`
	BlockSize                int           `mapstructure:"-"`
	BlockSizeRaw             string        `mapstructure:"-"`
	DedupMode                string        `mapstructure:"dedup"`
	CDCMin                   int           `mapstructure:"cdc_min"`
	CDCAvg                   int           `mapstructure:"cdc_avg"`
	CDCMax                   int           `mapstructure:"cdc_max"`
	DedupStrategy            string        `mapstructure:"dedup_strategy"`
	DedupStateFile           string        `mapstructure:"dedup_state_file"`
	BloomEntries             int           `mapstructure:"bloom_entries"`
	BloomFpRate              float64       `mapstructure:"bloom_fp_rate"`
	BloomMBits               uint          `mapstructure:"bloom_mbits"`
	ManifestPath             string        `mapstructure:"manifest_path"`
	ManifestTimeout          time.Duration `mapstructure:"manifest_timeout"`
	ManifestProgressInterval time.Duration `mapstructure:"manifest_progress_interval"`
	ManifestAllowMounted     bool          `mapstructure:"manifest_allow_mounted"`
	GRPCPort                 int           `mapstructure:"grpc_port"`
	GRPCListen               string        `mapstructure:"grpc_listen"`
	GRPCConnect              string        `mapstructure:"grpc_connect"`
	GRPCDialTimeout          time.Duration `mapstructure:"grpc_dial_timeout"`
	GRPCSetupTimeout         time.Duration `mapstructure:"grpc_setup_timeout"`
	HeartbeatInterval        time.Duration `mapstructure:"grpc_heartbeat_interval"`
	HeartbeatSendTimeout     time.Duration `mapstructure:"grpc_heartbeat_send_timeout"`
	TLSCert                  string        `mapstructure:"tls_cert"`
	TLSKey                   string        `mapstructure:"tls_key"`
	CACert                   string        `mapstructure:"ca_cert"`
	AllowInsecure            bool          `mapstructure:"allow_insecure"`
	Transport                string        `mapstructure:"transport"`
	TCPPort                  int           `mapstructure:"tcp_port"`
	TCPParallel              int           `mapstructure:"tcp_parallel"`
	TCPNotSentLowAt          int           `mapstructure:"tcp_lowat"`
	SyncInterval             string        `mapstructure:"sync_interval"`
	CheckpointInterval       time.Duration `mapstructure:"checkpoint_interval"`
	CheckpointBytesRaw       string        `mapstructure:"checkpoint_bytes"`
	SyncIntervalBytes        int           `mapstructure:"-"`
	CheckpointBytes          int           `mapstructure:"-"`
}

func FormatBlockSize(blockSize int) (string, error) {
	if blockSize < 0 {
		return "", fmt.Errorf("block size %d cannot be negative", blockSize)
	}
	if blockSize > math.MaxInt {
		return "", fmt.Errorf("block size %d overflows int", blockSize)
	}
	return sizeparse.FormatBytes(uint64(blockSize)), nil
}

func LZ4Level(level int) (uint32, error) {
	if level < 0 || level > int(math.MaxUint32) {
		return 0, fmt.Errorf("lz4 level %d out of range", level)
	}
	return uint32(level), nil
}

func (c *Config) HumanBlockSize() string {
	if c.BlockSize == 0 {
		return Auto
	}
	bs, err := FormatBlockSize(c.BlockSize)
	if err != nil {
		return ""
	}
	return bs
}

// BlockSizeBytes returns the configured block size in bytes.
// A zero or negative block size yields zero.
func (c *Config) BlockSizeBytes() uint64 {
	if c.BlockSize <= 0 {
		return 0
	}
	return uint64(c.BlockSize)
}

func DefaultConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	return &Config{
		Mode:                     "default",
		ApplyMode:                "",
		StdoutMode:               false,
		DryRun:                   false,
		Force:                    false,
		Offline:                  false,
		FSFreezeCommand:          "",
		FSThawCommand:            "",
		FreezeTimeout:            10 * time.Second,
		ThawTimeout:              10 * time.Second,
		Parallel:                 4,
		Concurrency:              0,
		ZeroCopy:                 false,
		ODirect:                  false,
		NumaPin:                  false,
		MaxRetries:               3,
		ResumeState:              "",
		ResumeToken:              "",
		DeviceUUID:               "",
		SSHHost:                  "localhost",
		SSHUser:                  "root",
		SSHPassword:              "",
		SSHKeyPath:               "",
		SSHPort:                  22,
		SSHTimeout:               10 * time.Second,
		SSHKeepAliveInterval:     30 * time.Second,
		SSHHostKey:               "",
		KnownHosts:               filepath.Join(homeDir, ".ssh", "known_hosts"),
		StrictHostKeyCheck:       true,
		LVMSyncPath:              "lvmsync",
		RemotePreScript:          "",
		RemotePostScript:         "",
		Compress:                 Auto,
		ZstdLevel:                1,
		LZ4Level:                 "fast",
		CompressConcurrency:      runtime.GOMAXPROCS(0),
		CompressThreshold:        0.9,
		Speed:                    "100MB",
		VerifyChecksum:           false,
		ChecksumAlgorithm:        Auto,
		Verbose:                  0,
		SkipSnapshotCreation:     false,
		SkipDiskCheck:            false,
		SnapshotSize:             "20%",
		SourceType:               "auto",
		DestType:                 "auto",
		VolumeGroup:              "",
		TargetVolumeGroup:        "",
		TargetVGCandidates:       []string{},
		LVMEscalation:            "sudo -n",
		LVMTimeout:               10 * time.Second,
		Progress:                 true,
		BlockSize:                0,
		BlockSizeRaw:             Auto,
		DedupMode:                "fixed",
		CDCMin:                   4 * 1024,
		CDCAvg:                   64 * 1024,
		CDCMax:                   1 * 1024 * 1024,
		DedupStrategy:            "none",
		DedupStateFile:           filepath.Join(homeDir, ".lvmsync_dedup"),
		BloomEntries:             1000000,
		BloomFpRate:              0.01,
		BloomMBits:               0,
		ManifestPath:             "",
		ManifestTimeout:          time.Minute,
		ManifestProgressInterval: 10 * time.Second,
		ManifestAllowMounted:     false,
		GRPCPort:                 8443,
		GRPCListen:               "",
		GRPCConnect:              "",
		GRPCDialTimeout:          5 * time.Second,
		GRPCSetupTimeout:         10 * time.Second,
		HeartbeatInterval:        30 * time.Second,
		HeartbeatSendTimeout:     5 * time.Second,
		TLSCert:                  "",
		TLSKey:                   "",
		CACert:                   "",
		AllowInsecure:            false,
		Transport:                "quic,h2,tcp+tls,ssh",
		TCPPort:                  0,
		TCPParallel:              1,
		TCPNotSentLowAt:          0,
		SyncInterval:             "1GB",
		CheckpointInterval:       0,
		CheckpointBytesRaw:       "1GB",
		SyncIntervalBytes:        1000000000,
		CheckpointBytes:          1000000000,
	}, nil
}
