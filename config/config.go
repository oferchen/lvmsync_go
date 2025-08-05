// config/config.go
package config

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/pierrec/lz4/v4"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"lvmsync_go/internal/compressiondetect"
	"lvmsync_go/lvm"
	"runtime"
)

var SupportedCompression = []string{"none", "lz4", "zstd", "auto"}
var SupportedDedupStrategies = []string{"auto", "checksum", "rolling_hash", "bloom"}

var (
	generalFlags     *pflag.FlagSet
	sshFlags         *pflag.FlagSet
	remoteFlags      *pflag.FlagSet
	dedupFlags       *pflag.FlagSet
	compressionFlags *pflag.FlagSet
	lvmFlags         *pflag.FlagSet
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
	Verbose              int      `mapstructure:"verbose"`
	SkipSnapshotCreation bool     `mapstructure:"skip_snapshot_creation"`
	SkipDiskCheck        bool     `mapstructure:"skip_disk_check"`
	SnapshotSize         string   `mapstructure:"snapshot_size"`
	VolumeGroup          string   `mapstructure:"volume_group"`
	TargetVolumeGroup    string   `mapstructure:"target_volume_group"`
	SourceVGCandidates   []string `mapstructure:"source_vgs"`
	TargetVGCandidates   []string `mapstructure:"target_vgs"`
	LVMEscalation        string   `mapstructure:"lvm_escalation"`
	Progress             bool     `mapstructure:"progress"`
	BlockSize            int      `mapstructure:"-"`
	BlockSizeRaw         string   `mapstructure:"-"`
	Deduplication        bool     `mapstructure:"deduplication"`
	DedupStrategy        string   `mapstructure:"dedup_strategy"`
	DedupStateFile       string   `mapstructure:"dedup_state_file"`
	BloomEntries         int      `mapstructure:"bloom_entries"`
	BloomFpRate          float64  `mapstructure:"bloom_fp_rate"`
}

func (c *Config) HumanBlockSize() string {
	return humanize.Bytes(uint64(c.BlockSize))
}

type ConfigBuilder struct {
	v        *viper.Viper
	defaults *Config
}

func (cb *ConfigBuilder) Build() (*Config, error) {
	var conf Config
	if err := cb.v.Unmarshal(&conf); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	conf.BlockSizeRaw = cb.getBlockSizeRaw()
	bs, err := cb.parseBytesOrFallback("block_size", cb.defaults.BlockSizeRaw)
	if err != nil {
		return nil, err
	}
	conf.BlockSize = bs
	sl, err := cb.parseBytesOrFallback("speed", cb.defaults.Speed)
	if err != nil {
		return nil, err
	}
	conf.SpeedLimit = sl

	if conf.CompressConcurrency <= 0 {
		conf.CompressConcurrency = runtime.GOMAXPROCS(0)
	}

	// Validate compression levels based on the resolved compression algorithm.
	resolved := conf.Compress
	if resolved == "auto" {
		resolved = compressiondetect.DetectOptimalCompression()
	}
	switch resolved {
	case "zstd":
		if conf.CompressLevel < 1 || conf.CompressLevel > 22 {
			return nil, fmt.Errorf("invalid zstd compression level: %d", conf.CompressLevel)
		}
	case "lz4":
		lvl := lz4.CompressionLevel(conf.CompressLevel)
		switch lvl {
		case lz4.Fast, lz4.Level1, lz4.Level2, lz4.Level3, lz4.Level4, lz4.Level5, lz4.Level6, lz4.Level7, lz4.Level8, lz4.Level9:
		// valid
		default:
			return nil, fmt.Errorf("invalid lz4 compression level: %d", conf.CompressLevel)
		}
	}

	return &conf, nil
}

func (cb *ConfigBuilder) parseBytesOrFallback(key, fallback string) (int, error) {
	raw := strings.ReplaceAll(cb.v.GetString(key), " ", "")
	if raw == "" {
		raw = fallback
	}
	val, err := humanize.ParseBytes(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", key, raw, err)
	}
	if val > uint64(math.MaxInt) {
		return 0, fmt.Errorf("%s value %q overflows int", key, raw)
	}
	return int(val), nil
}

func (cb *ConfigBuilder) getBlockSizeRaw() string {
	raw := strings.TrimSpace(cb.v.GetString("block_size"))
	if raw != "" {
		return raw
	}
	return cb.defaults.BlockSizeRaw
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
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
		Verbose:              0,
		SkipSnapshotCreation: false,
		SkipDiskCheck:        false,
		SnapshotSize:         "20%",
		VolumeGroup:          "vg0",
		TargetVolumeGroup:    "",
		SourceVGCandidates:   []string{},
		TargetVGCandidates:   []string{},
		LVMEscalation:        "sudo -n",
		Progress:             true,
		BlockSize:            4096,
		BlockSizeRaw:         "4KB",
		Deduplication:        false,
		DedupStrategy:        "auto",
		DedupStateFile:       filepath.Join(homeDir, ".lvmsync_dedup"),
		BloomEntries:         1000000,
		BloomFpRate:          0.01,
	}
}

func LoadConfig() (*Config, error) {
	defaultCfg := DefaultConfig()

	generalFlags = pflag.NewFlagSet("General Options", pflag.ExitOnError)
	sshFlags = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	remoteFlags = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	dedupFlags = pflag.NewFlagSet("Deduplication Options", pflag.ExitOnError)
	compressionFlags = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	lvmFlags = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)

	// General Options
	generalFlags.String("config", "", "Path to config YAML file")
	generalFlags.String("apply", defaultCfg.ApplyMode, "Apply mode: read change dump from file ('-' for STDIN) and apply to destination device")
	generalFlags.Bool("stdout", defaultCfg.StdoutMode, "Write change dump to STDOUT")
	generalFlags.Int("parallel", defaultCfg.Parallel, "Number of concurrent workers")
	generalFlags.Bool("zerocopy", defaultCfg.ZeroCopy, "Enable zero-copy transfers")
	generalFlags.Int("max_retries", defaultCfg.MaxRetries, "Maximum number of retries per block")
	generalFlags.String("resume", defaultCfg.ResumeState, "Path to resume state file")
	generalFlags.String("speed", defaultCfg.Speed, "Transfer speed limit")
	generalFlags.String("block_size", defaultCfg.BlockSizeRaw, "Block size for data transfer")
	generalFlags.CountP("verbose", "v", "Verbosity level")
	generalFlags.Bool("verify_checksum", defaultCfg.VerifyChecksum, "Enable checksum verification")
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
	dedupFlags.Bool("deduplication", defaultCfg.Deduplication, "Enable deduplication to avoid re-transferring unchanged blocks")
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
	lvmFlags.String("volume_group", defaultCfg.VolumeGroup, "Volume group name of the source LVM volume")
	lvmFlags.String("target_volume_group", defaultCfg.TargetVolumeGroup, "Volume group name of the target LVM volume")
	lvmFlags.StringSlice("source_vgs", defaultCfg.SourceVGCandidates, "Candidate source volume groups for auto-selection")
	lvmFlags.StringSlice("target_vgs", defaultCfg.TargetVGCandidates, "Candidate target volume groups for auto-selection")

	// Register flags and set usage
	pflag.CommandLine.AddFlagSet(generalFlags)
	pflag.CommandLine.AddFlagSet(sshFlags)
	pflag.CommandLine.AddFlagSet(remoteFlags)
	pflag.CommandLine.AddFlagSet(dedupFlags)
	pflag.CommandLine.AddFlagSet(compressionFlags)
	pflag.CommandLine.AddFlagSet(lvmFlags)
	pflag.Usage = printUsage
	pflag.Parse()

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("_", "."))
	v.SetEnvPrefix("LVMSYNC")

	if err := v.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}
	if cfgFile := v.GetString("config"); cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file %q: %w", cfgFile, err)
		}
	}

	builder := &ConfigBuilder{
		v:        v,
		defaults: defaultCfg,
	}
	conf, err := builder.Build()
	if err != nil {
		return nil, err
	}

	if conf.VolumeGroup == "" {
		vg, err := lvm.SelectVolumeGroupByFreeSpace(context.Background(), conf.SourceVGCandidates)
		if err != nil {
			return nil, fmt.Errorf("failed to select source volume group: %w", err)
		}
		conf.VolumeGroup = vg.Name
	}
	if conf.TargetVolumeGroup == "" && len(conf.TargetVGCandidates) > 0 {
		vg, err := lvm.SelectVolumeGroupByFreeSpace(context.Background(), conf.TargetVGCandidates)
		if err != nil {
			return nil, fmt.Errorf("failed to select target volume group: %w", err)
		}
		conf.TargetVolumeGroup = vg.Name
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
}
