// config/config.go
package config

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pierrec/lz4/v4"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"lvmsync_go/internal/compressiondetect"
	"lvmsync_go/internal/sizeparse"
	"lvmsync_go/lvm"
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
	SupportedChecksumAlgorithms = []string{"sha256", "blake3", "blake3-512"}
)

// FlagSets groups the flag sets for different configuration areas.
type FlagSets struct {
	General     *pflag.FlagSet
	SSH         *pflag.FlagSet
	Remote      *pflag.FlagSet
	Dedup       *pflag.FlagSet
	Compression *pflag.FlagSet
	LVM         *pflag.FlagSet
	GRPC        *pflag.FlagSet
	Transport   *pflag.FlagSet
	Serve       *pflag.FlagSet
}

// NewFlagSets constructs grouped flag sets using the provided defaults.
func NewFlagSets(cfg *Config) *FlagSets {
	return &FlagSets{
		General:     initGeneralFlags(cfg),
		SSH:         initSSHFlags(cfg),
		Remote:      initRemoteFlags(cfg),
		Dedup:       initDedupFlags(cfg),
		Compression: initCompressionFlags(cfg),
		LVM:         initLVMFlags(cfg),
		GRPC:        initGRPCFlags(cfg),
		Transport:   initTransportFlags(cfg),
		Serve:       initServeFlags(cfg),
	}
}

// All returns all flag sets in a stable order.
func (f *FlagSets) All() []*pflag.FlagSet {
	return []*pflag.FlagSet{
		f.General,
		f.SSH,
		f.Remote,
		f.Dedup,
		f.Compression,
		f.LVM,
		f.GRPC,
		f.Transport,
		f.Serve,
	}
}

type Config struct {
	ConfigFile            string        `mapstructure:"config"`
	ApplyMode             string        `mapstructure:"apply"`
	StdoutMode            bool          `mapstructure:"stdout"`
	Mode                  string        `mapstructure:"mode"`
	Parallel              int           `mapstructure:"parallel"`
	Concurrency           int           `mapstructure:"concurrency"`
	ZeroCopy              bool          `mapstructure:"zerocopy"`
	ODirect               bool          `mapstructure:"odirect"`
	NumaPin               bool          `mapstructure:"numa_pin"`
	MaxRetries            int           `mapstructure:"max_retries"`
	ResumeState           string        `mapstructure:"resume"`
	SSHHost               string        `mapstructure:"ssh_host"`
	SSHUser               string        `mapstructure:"ssh_user"`
	SSHKeyPath            string        `mapstructure:"ssh_key"`
	SSHPort               int           `mapstructure:"ssh_port"`
	SSHTimeout            time.Duration `mapstructure:"ssh_timeout"`
	SSHKeepAliveInterval  time.Duration `mapstructure:"ssh_keepalive"`
	KnownHosts            string        `mapstructure:"known_hosts"`
	StrictHostKeyCheck    bool          `mapstructure:"strict_host_key_checking"`
	LVMSyncPath           string        `mapstructure:"lvmsync_path"`
	RemotePreScript       string        `mapstructure:"remote_pre_script"`
	RemotePostScript      string        `mapstructure:"remote_post_script"`
	Compress              string        `mapstructure:"compress"`
	ZstdLevel             int           `mapstructure:"zstd_level"`
	LZ4Level              string        `mapstructure:"lz4_level"`
	CompressLevel         int           `mapstructure:"-"`
	CompressConcurrency   int           `mapstructure:"compress_concurrency"`
	CompressThreshold     float64       `mapstructure:"compress_threshold"`
	Speed                 string        `mapstructure:"speed"`
	SpeedLimit            int           `mapstructure:"-"`
	VerifyChecksum        bool          `mapstructure:"verify_checksum"`
	ChecksumAlgorithm     string        `mapstructure:"checksum_algorithm"`
	Verbose               int           `mapstructure:"verbose"`
	SkipSnapshotCreation  bool          `mapstructure:"skip_snapshot_creation"`
	SkipDiskCheck         bool          `mapstructure:"skip_disk_check"`
	SnapshotSize          string        `mapstructure:"snapshot_size"`
	VolumeGroup           string        `mapstructure:"volume_group"`
	TargetVolumeGroup     string        `mapstructure:"target_volume_group"`
	TargetVGCandidates    []string      `mapstructure:"target_vgs"`
	LVMEscalation         string        `mapstructure:"lvm_escalation"`
	LVMTimeout            time.Duration `mapstructure:"lvm_timeout"`
	Progress              bool          `mapstructure:"progress"`
	BlockSize             int           `mapstructure:"-"`
	BlockSizeRaw          string        `mapstructure:"-"`
	DedupMode             string        `mapstructure:"dedup"`
	CDCMin                int           `mapstructure:"cdc_min"`
	CDCAvg                int           `mapstructure:"cdc_avg"`
	CDCMax                int           `mapstructure:"cdc_max"`
	DedupStrategy         string        `mapstructure:"dedup_strategy"`
	DedupStateFile        string        `mapstructure:"dedup_state_file"`
	BloomEntries          int           `mapstructure:"bloom_entries"`
	BloomFpRate           float64       `mapstructure:"bloom_fp_rate"`
	BloomMBits            uint          `mapstructure:"bloom_mbits"`
	GRPCPort              int           `mapstructure:"grpc_port"`
	GRPCListen            string        `mapstructure:"grpc_listen"`
	GRPCConnect           string        `mapstructure:"grpc_connect"`
	GRPCDialTimeout       time.Duration `mapstructure:"grpc_dial_timeout"` // gRPC dial timeout
	HeartbeatInterval     time.Duration `mapstructure:"grpc_heartbeat_interval"`
	HeartbeatSendTimeout  time.Duration `mapstructure:"grpc_heartbeat_send_timeout"`
	TLSCert               string        `mapstructure:"tls_cert"`
	TLSKey                string        `mapstructure:"tls_key"`
	CACert                string        `mapstructure:"ca_cert"`
	AllowInsecure         bool          `mapstructure:"allow_insecure"`
	Transport             string        `mapstructure:"transport"`
	QUICListen            string        `mapstructure:"quic_listen"`
	QUICConnect           string        `mapstructure:"quic_connect"`
	TCPPort               int           `mapstructure:"tcp_port"`
	H2Port                int           `mapstructure:"h2_port"`
	SyncInterval          string        `mapstructure:"sync_interval"`
	CheckpointInterval    time.Duration `mapstructure:"checkpoint_interval"`
	QUICCongestionControl string        `mapstructure:"quic_cc"`
	SyncIntervalBytes     int           `mapstructure:"-"`

	Serve              bool          `mapstructure:"serve"`
	ServeListen        string        `mapstructure:"serve_listen"`
	ServeProtocol      string        `mapstructure:"serve_protocol"`
	ServeAlgorithm     string        `mapstructure:"serve_algorithm"`
	ServeTestSpace     string        `mapstructure:"serve_test_space"`
	ServePolicy        string        `mapstructure:"serve_policy"`
	ServeAcceptTimeout time.Duration `mapstructure:"serve_accept_timeout"`
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

type Builder struct {
	v        *viper.Viper
	defaults *Config
}

func (b *Builder) Build() (*Config, error) {
	var conf Config
	if err := b.v.Unmarshal(&conf); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := b.applyDefaults(&conf); err != nil {
		return nil, err
	}
	if err := b.validateCompression(&conf); err != nil {
		return nil, err
	}
	if err := b.finalizeConfig(&conf); err != nil {
		return nil, err
	}

	return &conf, nil
}

func (b *Builder) applyDefaults(conf *Config) error {
	if conf.Mode == "" {
		conf.Mode = b.defaults.Mode
	}
	if !b.v.IsSet("allow_insecure") {
		conf.AllowInsecure = b.defaults.AllowInsecure
	}
	if !b.v.IsSet("numa_pin") {
		conf.NumaPin = b.defaults.NumaPin
	}
	if conf.GRPCPort == 0 {
		conf.GRPCPort = b.defaults.GRPCPort
	}
	if conf.Transport == "" {
		conf.Transport = b.defaults.Transport
	}

	bs, raw, err := b.parseBlockSize()
	if err != nil {
		return err
	}
	conf.BlockSize = bs
	conf.BlockSizeRaw = raw
	if conf.ChecksumAlgorithm == "" {
		conf.ChecksumAlgorithm = b.defaults.ChecksumAlgorithm
	}
	sl, err := b.parseBytesOrFallback("speed", b.defaults.Speed)
	if err != nil {
		return err
	}
	conf.SpeedLimit = sl

	si, err := b.parseBytesOrFallback("sync_interval", b.defaults.SyncInterval)
	if err != nil {
		return err
	}
	conf.SyncIntervalBytes = si
	if conf.SyncInterval == "" {
		conf.SyncInterval = b.defaults.SyncInterval
	}

	if conf.CompressConcurrency <= 0 {
		conf.CompressConcurrency = runtime.GOMAXPROCS(0)
	}
	if conf.CompressThreshold <= 0 {
		conf.CompressThreshold = b.defaults.CompressThreshold
	}
	if conf.ZstdLevel == 0 {
		conf.ZstdLevel = b.defaults.ZstdLevel
	}
	if conf.LZ4Level == "" {
		conf.LZ4Level = b.defaults.LZ4Level
	}
	if conf.Mode == "throughput" {
		b.applyThroughput(conf)
	}
	return nil
}

func (b *Builder) applyThroughput(conf *Config) {
	if !b.v.IsSet("transport") {
		conf.Transport = "quic,h2,tcp+tls"
	}
	if !b.v.IsSet("parallel") {
		conf.Parallel = 8
	}
	if !b.v.IsSet("concurrency") {
		conf.Concurrency = 8
	}
	if !b.v.IsSet("dedup") {
		conf.DedupMode = "hybrid"
	}
	if !b.v.IsSet("block_size") {
		conf.BlockSize = 2 * 1024 * 1024
		conf.BlockSizeRaw = "2097152"
	}
	if !b.v.IsSet("cdc_min") {
		conf.CDCMin = 256 * 1024
	}
	if !b.v.IsSet("cdc_avg") {
		conf.CDCAvg = 2 * 1024 * 1024
	}
	if !b.v.IsSet("cdc_max") {
		conf.CDCMax = 8 * 1024 * 1024
	}
	if !b.v.IsSet("compress") {
		conf.Compress = Auto
	}
	if !b.v.IsSet("odirect") {
		conf.ODirect = true
	}
	if !b.v.IsSet("sync_interval") {
		conf.SyncInterval = "1GB"
		conf.SyncIntervalBytes = 1000000000
	}
	if !b.v.IsSet("checkpoint_interval") && conf.CheckpointInterval == 0 {
		conf.CheckpointInterval = 10 * time.Second
	}
	if conf.QUICCongestionControl == "" {
		conf.QUICCongestionControl = "bbr"
	}
}

func (b *Builder) validateCompression(conf *Config) error {
	resolved := conf.Compress
	if resolved == Auto {
		resolved = compressiondetect.DetectOptimalCompression()
	}
	switch resolved {
	case Zstd:
		if conf.ZstdLevel < 1 || conf.ZstdLevel > 5 {
			return fmt.Errorf("invalid zstd compression level: %d", conf.ZstdLevel)
		}
		conf.CompressLevel = conf.ZstdLevel
	case "lz4":
		switch strings.ToLower(conf.LZ4Level) {
		case "fast":
			conf.CompressLevel = int(lz4.Fast)
		case "hc":
			conf.CompressLevel = int(lz4.Level9)
		default:
			return fmt.Errorf("invalid lz4 compression level: %s", conf.LZ4Level)
		}
	}
	if conf.CompressThreshold <= 0 || conf.CompressThreshold > 1 {
		return fmt.Errorf("invalid compress threshold: %f", conf.CompressThreshold)
	}
	return nil
}

func (b *Builder) finalizeConfig(conf *Config) error {
	algo := strings.ToLower(conf.ChecksumAlgorithm)
	switch algo {
	case "sha256", "blake3", "blake3-512":
		conf.ChecksumAlgorithm = algo
	default:
		return fmt.Errorf("unsupported checksum algorithm: %s", conf.ChecksumAlgorithm)
	}

	if !conf.AllowInsecure && (conf.GRPCListen != "" || conf.GRPCConnect != "") {
		if conf.TLSCert == "" || conf.TLSKey == "" {
			return fmt.Errorf("tls_cert and tls_key must be specified unless allow_insecure is set")
		}
		if _, err := os.Stat(conf.TLSCert); err != nil {
			return fmt.Errorf("tls_cert: %w", err)
		}
		if _, err := os.Stat(conf.TLSKey); err != nil {
			return fmt.Errorf("tls_key: %w", err)
		}
		if conf.CACert != "" {
			if _, err := os.Stat(conf.CACert); err != nil {
				return fmt.Errorf("ca_cert: %w", err)
			}
		}
	}
	return nil
}

func (b *Builder) parseBytesOrFallback(key, fallback string) (int, error) {
	raw := strings.ReplaceAll(b.v.GetString(key), " ", "")
	if raw == "" {
		raw = fallback
	}
	val, isPercent, err := sizeparse.Parse(raw)
	if err != nil || isPercent {
		return 0, fmt.Errorf("invalid %s value %q: %w", key, raw, err)
	}
	u := uint64(val)
	if float64(u) != val || u > uint64(math.MaxInt) {
		return 0, fmt.Errorf("%s value %q overflows int", key, raw)
	}
	return int(u), nil
}

func (b *Builder) parseBlockSize() (int, string, error) {
	raw := strings.ReplaceAll(strings.TrimSpace(b.v.GetString("block_size")), " ", "")
	if raw == "" {
		raw = b.defaults.BlockSizeRaw
	}
	if strings.EqualFold(raw, Auto) {
		return 0, raw, nil
	}
	val, isPercent, err := sizeparse.Parse(raw)
	if err != nil || isPercent {
		return 0, "", fmt.Errorf("invalid block_size value %q: %w", raw, err)
	}
	u := uint64(val)
	if float64(u) != val || u > uint64(math.MaxInt) {
		return 0, "", fmt.Errorf("block_size value %q overflows int", raw)
	}
	return int(u), raw, nil
}

func DefaultConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	return &Config{
		Mode:                  "default",
		ApplyMode:             "",
		StdoutMode:            false,
		Parallel:              4,
		Concurrency:           0,
		ZeroCopy:              false,
		ODirect:               false,
		NumaPin:               false,
		MaxRetries:            3,
		ResumeState:           "",
		SSHHost:               "localhost",
		SSHUser:               "root",
		SSHKeyPath:            "",
		SSHPort:               22,
		SSHTimeout:            10 * time.Second,
		SSHKeepAliveInterval:  30 * time.Second,
		KnownHosts:            filepath.Join(homeDir, ".ssh", "known_hosts"),
		StrictHostKeyCheck:    true,
		LVMSyncPath:           "lvmsync",
		RemotePreScript:       "",
		RemotePostScript:      "",
		Compress:              Auto,
		ZstdLevel:             1,
		LZ4Level:              "fast",
		CompressConcurrency:   runtime.GOMAXPROCS(0),
		CompressThreshold:     0.9,
		Speed:                 "100MB",
		VerifyChecksum:        false,
		ChecksumAlgorithm:     "sha256",
		Verbose:               0,
		SkipSnapshotCreation:  false,
		SkipDiskCheck:         false,
		SnapshotSize:          "20%",
		VolumeGroup:           "",
		TargetVolumeGroup:     "",
		TargetVGCandidates:    []string{},
		LVMEscalation:         "sudo -n",
		LVMTimeout:            10 * time.Second,
		Progress:              true,
		BlockSize:             0,
		BlockSizeRaw:          Auto,
		DedupMode:             "fixed",
		CDCMin:                4 * 1024,
		CDCAvg:                64 * 1024,
		CDCMax:                1 * 1024 * 1024,
		DedupStrategy:         "none",
		DedupStateFile:        filepath.Join(homeDir, ".lvmsync_dedup"),
		BloomEntries:          1000000,
		BloomFpRate:           0.01,
		BloomMBits:            0,
		GRPCPort:              8443,
		GRPCListen:            "",
		GRPCConnect:           "",
		GRPCDialTimeout:       5 * time.Second,
		HeartbeatInterval:     30 * time.Second,
		HeartbeatSendTimeout:  5 * time.Second,
		TLSCert:               "",
		TLSKey:                "",
		CACert:                "",
		AllowInsecure:         false,
		Transport:             "quic,h2,tcp+tls,ssh",
		QUICListen:            "",
		QUICConnect:           "",
		TCPPort:               0,
		H2Port:                0,
		SyncInterval:          "1GB",
		CheckpointInterval:    0,
		QUICCongestionControl: "",
		SyncIntervalBytes:     1000000000,
		Serve:                 false,
		ServeListen:           ":9000",
		ServeProtocol:         "lvmsync",
		ServeAlgorithm:        "sha256",
		ServeTestSpace:        "",
		ServePolicy:           "accept",
		ServeAcceptTimeout:    30 * time.Second,
	}, nil
}

func initGeneralFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("General Options", pflag.ExitOnError)
	fs.String("config", "", "Path to config YAML file")
	fs.String("apply", cfg.ApplyMode, "Apply mode: read change dump from file ('-' for STDIN) and apply to destination device")
	fs.Bool("stdout", cfg.StdoutMode, "Write change dump to STDOUT")
	fs.String("mode", cfg.Mode, "Preset mode: default or throughput")
	fs.Int("parallel", cfg.Parallel, "Number of concurrent workers")
	fs.Bool("zerocopy", cfg.ZeroCopy, "Enable zero-copy transfers")
	fs.Bool("odirect", cfg.ODirect, "Use O_DIRECT for device I/O when possible")
	fs.Bool("numa_pin", cfg.NumaPin, "Pin worker goroutines to device NUMA node")
	fs.Int("max_retries", cfg.MaxRetries, "Maximum number of retries per block")
	fs.String("resume", cfg.ResumeState, "Path to resume state file")
	fs.String("speed", cfg.Speed, "Transfer speed limit")
	fs.String("sync_interval", cfg.SyncInterval, "Bytes between fdatasync calls")
	fs.Duration("checkpoint_interval", cfg.CheckpointInterval, "Duration between checkpoints")
	fs.String("block_size", cfg.BlockSizeRaw, "Block size for data transfer; specify 'auto' or 0 for automatic detection")
	fs.CountP("verbose", "v", "Verbosity level")
	fs.Bool("verify_checksum", cfg.VerifyChecksum, "Enable checksum verification")
	fs.String("checksum_algorithm", cfg.ChecksumAlgorithm, fmt.Sprintf("Checksum algorithm: %v", SupportedChecksumAlgorithms))
	fs.Bool("progress", cfg.Progress, "Show progress during transfer")
	return fs
}

func initSSHFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	fs.String("ssh_host", cfg.SSHHost, "SSH host")
	fs.String("ssh_user", cfg.SSHUser, "SSH username")
	fs.String("ssh_key", cfg.SSHKeyPath, "Path to SSH private key or use agent")
	fs.Int("ssh_port", cfg.SSHPort, "SSH port")
	fs.Duration("ssh_timeout", cfg.SSHTimeout, "SSH connection timeout")
	fs.Duration("ssh_keepalive", cfg.SSHKeepAliveInterval, "SSH keepalive interval")
	fs.String("known_hosts", cfg.KnownHosts, "Path to known_hosts file")
	fs.Bool("strict_host_key_checking", cfg.StrictHostKeyCheck, "Require host keys to be present in known_hosts")
	return fs
}

func initRemoteFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	fs.String("lvmsync_path", cfg.LVMSyncPath, "Remote command to run")
	fs.String("remote_pre_script", cfg.RemotePreScript, "Remote script to run before transfer")
	fs.String("remote_post_script", cfg.RemotePostScript, "Remote script to run after transfer")
	return fs
}

func initDedupFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Deduplication Options", pflag.ExitOnError)
	fs.String("dedup", cfg.DedupMode, fmt.Sprintf("Deduplication mode: %v", SupportedDedupModes))
	fs.Int("cdc_min", cfg.CDCMin, "Minimum chunk size for CDC")
	fs.Int("cdc_avg", cfg.CDCAvg, "Average chunk size for CDC")
	fs.Int("cdc_max", cfg.CDCMax, "Maximum chunk size for CDC")
	fs.String("dedup_strategy", cfg.DedupStrategy, fmt.Sprintf("Deduplication strategy: %v", SupportedDedupStrategies))
	fs.String("dedup_state_file", cfg.DedupStateFile, "Path to deduplication state file")
	fs.Int("bloom_entries", cfg.BloomEntries, "Estimated number of entries for bloom filter")
	fs.Float64("bloom_fp_rate", cfg.BloomFpRate, "False positive rate for bloom filter")
	fs.Uint("bloom_mbits", cfg.BloomMBits, "Bloom filter m bits power")
	return fs
}

func initCompressionFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	fs.String("compress", cfg.Compress, fmt.Sprintf("Compression type, options: %v", SupportedCompression))
	fs.Int("zstd_level", cfg.ZstdLevel, "Zstd compression level (1-5)")
	fs.String("lz4_level", cfg.LZ4Level, "LZ4 compression level: fast or hc")
	fs.Int("compress_concurrency", cfg.CompressConcurrency, "Compression concurrency (0 to use GOMAXPROCS)")
	fs.Float64("compress_threshold", cfg.CompressThreshold, "Skip compression when estimated ratio exceeds this value")
	return fs
}

func initLVMFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
	fs.Bool("skip_snapshot_creation", cfg.SkipSnapshotCreation, "Skip automatic snapshot creation")
	fs.Bool("skip_disk_check", cfg.SkipDiskCheck, "Skip disk space check before snapshot creation")
	fs.String("snapshot_size", cfg.SnapshotSize, "Snapshot size (e.g., '20G' or '20%')")
	fs.String("lvm_escalation", cfg.LVMEscalation, "Command used to escalate privileges for LVM commands")
	fs.Duration("lvm_timeout", cfg.LVMTimeout, "Timeout for LVM operations")
	fs.String("volume_group", cfg.VolumeGroup, "Source volume group; derived from the source device path when empty")
	fs.String("target_volume_group", cfg.TargetVolumeGroup, "Volume group name of the target LVM volume")
	fs.StringSlice("target_vgs", cfg.TargetVGCandidates, "Candidate target volume groups for auto-selection")
	return fs
}

func initGRPCFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("gRPC Options", pflag.ExitOnError)
	fs.Int("grpc_port", cfg.GRPCPort, "gRPC port to listen on")
	fs.String("grpc_listen", cfg.GRPCListen, "gRPC listen address")
	fs.String("grpc_connect", cfg.GRPCConnect, "gRPC server address to connect to")
	fs.Duration("grpc_dial_timeout", cfg.GRPCDialTimeout, "gRPC dial timeout")
	fs.Duration("grpc_heartbeat_interval", cfg.HeartbeatInterval, "gRPC heartbeat interval")
	fs.Duration("grpc_heartbeat_send_timeout", cfg.HeartbeatSendTimeout, "gRPC heartbeat send timeout")
	fs.String("tls_cert", cfg.TLSCert, "TLS certificate file")
	fs.String("tls_key", cfg.TLSKey, "TLS key file")
	fs.String("ca_cert", cfg.CACert, "CA certificate file")
	fs.Bool("allow_insecure", cfg.AllowInsecure, "Allow insecure (no TLS)")
	return fs
}

func initTransportFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
	fs.String("transport", cfg.Transport, "Ordered transports to try (e.g. 'quic,h2,tcp+tls,ssh')")
	fs.String("quic_listen", cfg.QUICListen, "QUIC listen address")
	fs.String("quic_connect", cfg.QUICConnect, "QUIC connect address")
	fs.String("quic_cc", cfg.QUICCongestionControl, "QUIC congestion control algorithm")
	fs.Int("concurrency", cfg.Concurrency, "Stream concurrency (0 to autotune)")
	fs.Int("tcp_port", cfg.TCPPort, "TCP+TLS port")
	fs.Int("h2_port", cfg.H2Port, "HTTP/2 TLS port")
	return fs
}

func initServeFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Serve Options", pflag.ExitOnError)
	fs.Bool("serve", cfg.Serve, "Run in serve mode")
	fs.String("serve_listen", cfg.ServeListen, "QUIC listen address")
	fs.String("serve_protocol", cfg.ServeProtocol, "Protocol to negotiate")
	fs.String("serve_algorithm", cfg.ServeAlgorithm, "Algorithm to negotiate")
	fs.String("serve_test_space", cfg.ServeTestSpace, "Test-space option")
	fs.String("serve_policy", cfg.ServePolicy, "Transfer policy")
	fs.Duration("serve_accept_timeout", cfg.ServeAcceptTimeout, "Timeout for accepting connection and stream")
	return fs
}

func printFlagSetUsage(out io.Writer, fs *pflag.FlagSet) {
	if fs == nil {
		return
	}
	fmt.Fprintln(out, fs.Name()+":")
	fs.SetOutput(out)
	fs.PrintDefaults()
	fmt.Fprintln(out)
}

func registerFlags(flagSets *FlagSets) {
	for _, fs := range flagSets.All() {
		pflag.CommandLine.AddFlagSet(fs)
	}
	pflag.CommandLine.Usage = func() {
		out := pflag.CommandLine.Output()
		fmt.Fprintln(out, "Usage of", os.Args[0]+":")
		fmt.Fprintln(out)
		for _, fs := range flagSets.All() {
			printFlagSetUsage(out, fs)
		}
	}
	pflag.Usage = pflag.CommandLine.Usage
}

func buildViper(flagSets *FlagSets) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetEnvPrefix("LVMSYNC")
	v.AutomaticEnv()
	keys := []string{
		"transport",
		"quic_listen",
		"quic_connect",
		"quic_cc",
		"concurrency",
		"tcp_port",
		"h2_port",
	}
	for _, k := range keys {
		if err := v.BindEnv(k); err != nil {
			return nil, err
		}
	}
	for _, fs := range flagSets.All() {
		if err := v.BindPFlags(fs); err != nil {
			return nil, err
		}
	}
	if cfgFile := v.GetString("config"); cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file %q: %w", cfgFile, err)
		}
	}
	return v, nil
}

func LoadConfig(flagSets *FlagSets, defaults *Config) (*Config, error) {
	registerFlags(flagSets)
	pflag.Parse()

	v, err := buildViper(flagSets)
	if err != nil {
		return nil, err
	}

	builder := &Builder{
		v:        v,
		defaults: defaults,
	}
	conf, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return conf, nil
}

// Validate verifies configuration values using the real OS euid.
func (c *Config) Validate() error { return c.ValidateWith(os.Geteuid) }

// ValidateWith verifies configuration values using the provided geteuid function.
func (c *Config) ValidateWith(geteuid func() int) error {
	if c.SSHKeepAliveInterval <= 0 {
		return fmt.Errorf("ssh keepalive interval must be > 0")
	}
	if c.GRPCDialTimeout <= 0 {
		return fmt.Errorf("grpc dial timeout must be > 0")
	}
	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("grpc heartbeat interval must be > 0")
	}
	if c.HeartbeatSendTimeout <= 0 {
		return fmt.Errorf("grpc heartbeat send timeout must be > 0")
	}
	if c.VolumeGroup != "" {
		ctx, cancel := context.WithTimeout(context.Background(), c.LVMTimeout)
		defer cancel()
		if _, err := lvm.GetVolumeGroupFreeSpace(ctx, c.VolumeGroup); err != nil {
			return fmt.Errorf("volume group %q does not exist or is inaccessible: %w", c.VolumeGroup, err)
		}
	}
	if c.TargetVolumeGroup != "" {
		ctx, cancel := context.WithTimeout(context.Background(), c.LVMTimeout)
		defer cancel()
		if _, err := lvm.GetVolumeGroupFreeSpace(ctx, c.TargetVolumeGroup); err != nil {
			return fmt.Errorf("target volume group %q does not exist or is inaccessible: %w", c.TargetVolumeGroup, err)
		}
	}
	if geteuid() != 0 {
		parts := strings.Fields(c.LVMEscalation)
		if len(parts) == 0 {
			return fmt.Errorf("lvm escalation command is empty")
		}
		if _, err := findInPath(parts[0]); err != nil {
			return fmt.Errorf("lvm escalation command %q not found: %w", parts[0], err)
		}
	}
	return nil
}

func findInPath(name string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		info, err := os.Stat(name)
		if err != nil {
			return "", err
		}
		if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("%s is not executable", name)
		}
		return name, nil
	}
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return full, nil
		}
	}
	return "", fmt.Errorf("executable %q not found in $PATH", name)
}
