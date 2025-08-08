// config/config.go
package config

import (
	"context"
	"fmt"
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

var SupportedCompression = []string{"none", "lz4", "zstd", "auto"}
var SupportedDedupStrategies = []string{"none", "auto", "checksum", "rolling_hash", "bloom"}
var SupportedChecksumAlgorithms = []string{"sha256", "blake3", "blake3-512"}

var (
	generalFlags     *pflag.FlagSet
	sshFlags         *pflag.FlagSet
	remoteFlags      *pflag.FlagSet
	dedupFlags       *pflag.FlagSet
	compressionFlags *pflag.FlagSet
	lvmFlags         *pflag.FlagSet
	grpcFlags        *pflag.FlagSet
)

var getEuid = os.Geteuid

type Config struct {
	ConfigFile           string        `mapstructure:"config"`
	ApplyMode            string        `mapstructure:"apply"`
	StdoutMode           bool          `mapstructure:"stdout"`
	Parallel             int           `mapstructure:"parallel"`
	ZeroCopy             bool          `mapstructure:"zerocopy"`
	MaxRetries           int           `mapstructure:"max_retries"`
	ResumeState          string        `mapstructure:"resume"`
	SSHUser              string        `mapstructure:"ssh_user"`
	SSHKeyPath           string        `mapstructure:"ssh_key"`
	SSHPort              int           `mapstructure:"ssh_port"`
	SSHTimeout           time.Duration `mapstructure:"ssh_timeout"`
	SSHKeepAliveInterval time.Duration `mapstructure:"ssh_keepalive"`
	KnownHosts           string        `mapstructure:"known_hosts"`
	StrictHostKeyCheck   bool          `mapstructure:"strict_host_key_checking"`
	LVMSyncPath          string        `mapstructure:"lvmsync_path"`
	RemotePreScript      string        `mapstructure:"remote_pre_script"`
	RemotePostScript     string        `mapstructure:"remote_post_script"`
	Compress             string        `mapstructure:"compress"`
	// For LZ4 use lz4.Fast or lz4.Level1 through lz4.Level9; ZSTD accepts levels 1-22.
	CompressLevel        int      `mapstructure:"compress_level"`
	CompressConcurrency  int      `mapstructure:"compress_concurrency"`
	Speed                string   `mapstructure:"speed"`
	SpeedLimit           int      `mapstructure:"-"`
	VerifyChecksum       bool     `mapstructure:"verify_checksum"`
	ChecksumAlgorithm    string   `mapstructure:"checksum_algorithm"`
	Verbose              int      `mapstructure:"verbose"`
	SkipSnapshotCreation bool     `mapstructure:"skip_snapshot_creation"`
	SkipDiskCheck        bool     `mapstructure:"skip_disk_check"`
	SnapshotSize         string   `mapstructure:"snapshot_size"`
	VolumeGroup          string   `mapstructure:"volume_group"`
	TargetVolumeGroup    string   `mapstructure:"target_volume_group"`
	TargetVGCandidates   []string `mapstructure:"target_vgs"`
	LVMEscalation        string   `mapstructure:"lvm_escalation"`
	Progress             bool     `mapstructure:"progress"`
	BlockSize            int      `mapstructure:"-"`
	BlockSizeRaw         string   `mapstructure:"-"`
	DedupStrategy        string   `mapstructure:"dedup_strategy"`
	DedupStateFile       string   `mapstructure:"dedup_state_file"`
	BloomEntries         int      `mapstructure:"bloom_entries"`
	BloomFpRate          float64  `mapstructure:"bloom_fp_rate"`
	GRPCPort             int      `mapstructure:"grpc_port"`
	TLSCert              string   `mapstructure:"tls_cert"`
	TLSKey               string   `mapstructure:"tls_key"`
	CACert               string   `mapstructure:"ca_cert"`
	AllowInsecure        bool     `mapstructure:"allow_insecure"`
	SudoPath             string   `mapstructure:"sudo_path"`
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
		return "auto"
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
	if !b.v.IsSet("allow_insecure") {
		conf.AllowInsecure = b.defaults.AllowInsecure
	}
	if conf.GRPCPort == 0 {
		conf.GRPCPort = b.defaults.GRPCPort
	}
	if conf.SudoPath == "" {
		conf.SudoPath = b.defaults.SudoPath
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

	if conf.CompressConcurrency <= 0 {
		conf.CompressConcurrency = runtime.GOMAXPROCS(0)
	}
	return nil
}

func (b *Builder) validateCompression(conf *Config) error {
	resolved := conf.Compress
	if resolved == "auto" {
		resolved = compressiondetect.DetectOptimalCompression()
	}
	switch resolved {
	case "zstd":
		if conf.CompressLevel < 1 || conf.CompressLevel > 22 {
			return fmt.Errorf("invalid zstd compression level: %d", conf.CompressLevel)
		}
	case "lz4":
		if conf.CompressLevel < int(lz4.Fast) || conf.CompressLevel > int(lz4.Level9) {
			return fmt.Errorf("invalid lz4 compression level: %d", conf.CompressLevel)
		}
		_ = lz4.CompressionLevel(conf.CompressLevel)
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

	if !conf.AllowInsecure {
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
	if strings.EqualFold(raw, "auto") {
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
		ApplyMode:            "",
		StdoutMode:           false,
		Parallel:             4,
		ZeroCopy:             false,
		MaxRetries:           3,
		ResumeState:          "",
		SSHUser:              "root",
		SSHKeyPath:           "",
		SSHPort:              22,
		SSHTimeout:           10 * time.Second,
		SSHKeepAliveInterval: 30 * time.Second,
		KnownHosts:           filepath.Join(homeDir, ".ssh", "known_hosts"),
		StrictHostKeyCheck:   true,
		LVMSyncPath:          "lvmsync",
		RemotePreScript:      "",
		RemotePostScript:     "",
		Compress:             "auto",
		CompressLevel:        3,
		CompressConcurrency:  runtime.GOMAXPROCS(0),
		Speed:                "100MB",
		VerifyChecksum:       false,
		ChecksumAlgorithm:    "sha256",
		Verbose:              0,
		SkipSnapshotCreation: false,
		SkipDiskCheck:        false,
		SnapshotSize:         "20%",
		VolumeGroup:          "",
		TargetVolumeGroup:    "",
		TargetVGCandidates:   []string{},
		LVMEscalation:        "sudo -n",
		Progress:             true,
		BlockSize:            0,
		BlockSizeRaw:         "auto",
		DedupStrategy:        "none",
		DedupStateFile:       filepath.Join(homeDir, ".lvmsync_dedup"),
		BloomEntries:         1000000,
		BloomFpRate:          0.01,
		GRPCPort:             8443,
		TLSCert:              "",
		TLSKey:               "",
		CACert:               "",
		AllowInsecure:        true,
		SudoPath:             "/usr/bin/sudo",
	}, nil
}

func LoadConfig() (*Config, error) {
	defaultCfg, err := DefaultConfig()
	if err != nil {
		return nil, err
	}

	generalFlags = pflag.NewFlagSet("General Options", pflag.ExitOnError)
	sshFlags = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	remoteFlags = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	dedupFlags = pflag.NewFlagSet("Deduplication Options", pflag.ExitOnError)
	compressionFlags = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	lvmFlags = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
	grpcFlags = pflag.NewFlagSet("gRPC Options", pflag.ExitOnError)

	// General Options
	generalFlags.String("config", "", "Path to config YAML file")
	generalFlags.String("apply", defaultCfg.ApplyMode, "Apply mode: read change dump from file ('-' for STDIN) and apply to destination device")
	generalFlags.Bool("stdout", defaultCfg.StdoutMode, "Write change dump to STDOUT")
	generalFlags.Int("parallel", defaultCfg.Parallel, "Number of concurrent workers")
	generalFlags.Bool("zerocopy", defaultCfg.ZeroCopy, "Enable zero-copy transfers")
	generalFlags.Int("max_retries", defaultCfg.MaxRetries, "Maximum number of retries per block")
	generalFlags.String("resume", defaultCfg.ResumeState, "Path to resume state file")
	generalFlags.String("speed", defaultCfg.Speed, "Transfer speed limit")
	generalFlags.String("block_size", defaultCfg.BlockSizeRaw, "Block size for data transfer; specify 'auto' or 0 for automatic detection")
	generalFlags.CountP("verbose", "v", "Verbosity level")
	generalFlags.Bool("verify_checksum", defaultCfg.VerifyChecksum, "Enable checksum verification")
	generalFlags.String("checksum_algorithm", defaultCfg.ChecksumAlgorithm, fmt.Sprintf("Checksum algorithm: %v", SupportedChecksumAlgorithms))
	generalFlags.Bool("progress", defaultCfg.Progress, "Show progress during transfer")

	// SSH Options
	sshFlags.String("ssh_user", defaultCfg.SSHUser, "SSH username")
	sshFlags.String("ssh_key", defaultCfg.SSHKeyPath, "Path to SSH private key or use agent")
	sshFlags.Int("ssh_port", defaultCfg.SSHPort, "SSH port")
	sshFlags.Duration("ssh_timeout", defaultCfg.SSHTimeout, "SSH connection timeout")
	sshFlags.Duration("ssh_keepalive", defaultCfg.SSHKeepAliveInterval, "SSH keepalive interval")
	sshFlags.String("known_hosts", defaultCfg.KnownHosts, "Path to known_hosts file")
	sshFlags.Bool("stricthostkeychecking", defaultCfg.StrictHostKeyCheck, "Enable SSH StrictHostKeyChecking")

	// Remote Options
	remoteFlags.String("lvmsync_path", defaultCfg.LVMSyncPath, "Remote command to run")
	remoteFlags.String("remote_pre_script", defaultCfg.RemotePreScript, "Remote script to run before transfer")
	remoteFlags.String("remote_post_script", defaultCfg.RemotePostScript, "Remote script to run after transfer")

	// Deduplication Options
	dedupFlags.String("dedup_strategy", defaultCfg.DedupStrategy, fmt.Sprintf("Deduplication strategy: %v", SupportedDedupStrategies))
	dedupFlags.String("dedup_state_file", defaultCfg.DedupStateFile, "Path to deduplication state file")
	dedupFlags.Int("bloom_entries", defaultCfg.BloomEntries, "Estimated number of entries for bloom filter")
	dedupFlags.Float64("bloom_fp_rate", defaultCfg.BloomFpRate, "False positive rate for bloom filter")

	// Compression Options
	compressionFlags.String("compress", defaultCfg.Compress, fmt.Sprintf("Compression type, options: %v", SupportedCompression))
	compressionFlags.Int("compress_level", defaultCfg.CompressLevel, "Compression level. LZ4 accepts lz4.Fast or lz4.Level1..lz4.Level9; ZSTD accepts 1-22")
	compressionFlags.Int("compress_concurrency", defaultCfg.CompressConcurrency, "Compression concurrency (0 to use GOMAXPROCS)")

	// LVM Options
	lvmFlags.Bool("skip_snapshot_creation", defaultCfg.SkipSnapshotCreation, "Skip automatic snapshot creation")
	lvmFlags.Bool("skip_disk_check", defaultCfg.SkipDiskCheck, "Skip disk space check before snapshot creation")
	lvmFlags.String("snapshot_size", defaultCfg.SnapshotSize, "Snapshot size (e.g., '20G' or '20%')")
	lvmFlags.String("lvm_escalation", defaultCfg.LVMEscalation, "Command used to escalate privileges for LVM commands")
	lvmFlags.String("volume_group", defaultCfg.VolumeGroup, "Source volume group; derived from the source device path when empty")
	lvmFlags.String("target_volume_group", defaultCfg.TargetVolumeGroup, "Volume group name of the target LVM volume")
	lvmFlags.StringSlice("target_vgs", defaultCfg.TargetVGCandidates, "Candidate target volume groups for auto-selection")

	// gRPC Options
	grpcFlags.Int("grpc_port", defaultCfg.GRPCPort, "gRPC port to listen on")
	grpcFlags.String("tls_cert", defaultCfg.TLSCert, "TLS certificate file")
	grpcFlags.String("tls_key", defaultCfg.TLSKey, "TLS key file")
	grpcFlags.String("ca_cert", defaultCfg.CACert, "CA certificate file")
	grpcFlags.Bool("allow_insecure", defaultCfg.AllowInsecure, "Allow insecure (no TLS)")
	grpcFlags.String("sudo_path", defaultCfg.SudoPath, "Path to sudo executable")

	// Register flags and set usage
	pflag.CommandLine.AddFlagSet(generalFlags)
	pflag.CommandLine.AddFlagSet(sshFlags)
	pflag.CommandLine.AddFlagSet(remoteFlags)
	pflag.CommandLine.AddFlagSet(dedupFlags)
	pflag.CommandLine.AddFlagSet(compressionFlags)
	pflag.CommandLine.AddFlagSet(lvmFlags)
	pflag.CommandLine.AddFlagSet(grpcFlags)
	pflag.Usage = printUsage
	pflag.Parse()

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("_", "."))
	v.SetEnvPrefix("LVMSYNC")

	if err = v.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}
	if cfgFile := v.GetString("config"); cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err = v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file %q: %w", cfgFile, err)
		}
	}

	builder := &Builder{
		v:        v,
		defaults: defaultCfg,
	}
	conf, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return conf, nil
}
func (c *Config) Validate() error {
	if c.VolumeGroup != "" {
		if _, err := lvm.GetVolumeGroupFreeSpace(context.Background(), c.VolumeGroup); err != nil {
			return fmt.Errorf("volume group %q does not exist or is inaccessible: %w", c.VolumeGroup, err)
		}
	}
	if c.TargetVolumeGroup != "" {
		if _, err := lvm.GetVolumeGroupFreeSpace(context.Background(), c.TargetVolumeGroup); err != nil {
			return fmt.Errorf("target volume group %q does not exist or is inaccessible: %w", c.TargetVolumeGroup, err)
		}
	}
	if getEuid() != 0 {
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
		if info.IsDir() || info.Mode().Perm()&0111 == 0 {
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
		if !info.IsDir() && info.Mode().Perm()&0111 != 0 {
			return full, nil
		}
	}
	return "", fmt.Errorf("executable %q not found in $PATH", name)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <snapshot|lvm device> <destination>\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "General Options:\n")
	generalFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nSSH Options:\n")
	sshFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nRemote Options:\n")
	remoteFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nDeduplication Options:\n")
	dedupFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nCompression Options:\n")
	compressionFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nLVM Options:\n")
	lvmFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\ngRPC Options:\n")
	grpcFlags.PrintDefaults()
}
