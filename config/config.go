// config/config.go
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var SupportedCompression = []string{"none", "lz4", "zstd", "auto"}

type Config struct {
	ConfigFile           string `mapstructure:"config"`
	ApplyMode            string `mapstructure:"apply"`
	StdoutMode           bool   `mapstructure:"stdout"`
	Parallel             int    `mapstructure:"parallel"`
	ZeroCopy             bool   `mapstructure:"zerocopy"`
	MaxRetries           int    `mapstructure:"max_retries"`
	ResumeState          string `mapstructure:"resume"`
	SSHUser              string `mapstructure:"ssh_user"`
	SSHKeyPath           string `mapstructure:"ssh_key"`
	SSHPort              int    `mapstructure:"ssh_port"`
	KnownHosts           string `mapstructure:"known_hosts"`
	StrictHostKeyCheck   bool   `mapstructure:"strict_host_key_checking"`
	LVMSyncPath          string `mapstructure:"lvmsync_path"`
	RemotePreScript      string `mapstructure:"remote_pre_script"`
	RemotePostScript     string `mapstructure:"remote_post_script"`
	Compress             string `mapstructure:"compress"`
	CompressLevel        int    `mapstructure:"compress_level"`
	Speed                string `mapstructure:"speed"`
	SpeedLimit           int    `mapstructure:"-"`
	VerifyChecksum       bool   `mapstructure:"verify_checksum"`
	Verbose              int    `mapstructure:"verbose"`
	SkipSnapshotCreation bool   `mapstructure:"skip_snapshot_creation"`
	SkipDiskCheck        bool   `mapstructure:"skip_disk_check"`
	SnapshotSize         string `mapstructure:"snapshot_size"`
	VolumeGroup          string `mapstructure:"volume_group"`
	LVMEscalation        string `mapstructure:"lvm_escalation"`
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
		KnownHosts:           filepath.Join(homeDir, ".ssh", "known_hosts"),
		StrictHostKeyCheck:   true,
		LVMSyncPath:          "lvmsync",
		RemotePreScript:      "",
		RemotePostScript:     "",
		Compress:             "lz4",
		CompressLevel:        3,
		Speed:                "100MB",
		VerifyChecksum:       false,
		Verbose:              0,
		SkipSnapshotCreation: false,
		SkipDiskCheck:        false,
		SnapshotSize:         "20%",
		VolumeGroup:          "vg0",
		LVMEscalation:        "sudo -n",
	}
}

func LoadConfig() (*Config, error) {
	defaultCfg := DefaultConfig()

	generalFlags := pflag.NewFlagSet("General Options", pflag.ExitOnError)
	sshFlags := pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	remoteFlags := pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	compressionFlags := pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	lvmFlags := pflag.NewFlagSet("LVM Options", pflag.ExitOnError)

	// General Options.
	generalFlags.String("config", "", "Path to config YAML file")
	generalFlags.String("apply", defaultCfg.ApplyMode, "Apply mode: read change dump from file ('-' for STDIN) and apply to destination device")
	generalFlags.Bool("stdout", defaultCfg.StdoutMode, fmt.Sprintf("Write change dump to STDOUT (default: %v)", defaultCfg.StdoutMode))
	generalFlags.Int("parallel", defaultCfg.Parallel, fmt.Sprintf("Number of concurrent workers (default: %d)", defaultCfg.Parallel))
	generalFlags.Bool("zerocopy", defaultCfg.ZeroCopy, fmt.Sprintf("Enable zero-copy transfers (default: %v)", defaultCfg.ZeroCopy))
	generalFlags.Int("max_retries", defaultCfg.MaxRetries, fmt.Sprintf("Maximum number of retries per block (default: %d)", defaultCfg.MaxRetries))
	generalFlags.String("resume", defaultCfg.ResumeState, fmt.Sprintf("Path to resume state file (default: %q)", defaultCfg.ResumeState))
	generalFlags.String("speed", defaultCfg.Speed, fmt.Sprintf("Speed limit (default: %s)", defaultCfg.Speed))
	generalFlags.CountP("verbose", "v", "Set verbosity level (e.g. -v, -vv, -vvv)")
	generalFlags.Bool("verify_checksum", defaultCfg.VerifyChecksum, fmt.Sprintf("Enable checksum verification (default: %v)", defaultCfg.VerifyChecksum))

	// SSH Options.
	sshFlags.String("ssh_user", defaultCfg.SSHUser, fmt.Sprintf("SSH username (default: %q)", defaultCfg.SSHUser))
	sshFlags.String("ssh_key", defaultCfg.SSHKeyPath, fmt.Sprintf("Path to SSH private key or use agent (default: %q)", defaultCfg.SSHKeyPath))
	sshFlags.Int("ssh_port", defaultCfg.SSHPort, fmt.Sprintf("SSH port (default: %d)", defaultCfg.SSHPort))
	sshFlags.String("known_hosts", defaultCfg.KnownHosts, fmt.Sprintf("Path to known_hosts file (default: %q)", defaultCfg.KnownHosts))
	sshFlags.Bool("stricthostkeychecking", defaultCfg.StrictHostKeyCheck, fmt.Sprintf("Enable SSH StrictHostKeyChecking (default: %v)", defaultCfg.StrictHostKeyCheck))

	// Remote Options.
	remoteFlags.String("lvmsync_path", defaultCfg.LVMSyncPath, fmt.Sprintf("Remote command to run (default: %q)", defaultCfg.LVMSyncPath))
	remoteFlags.String("remote_pre_script", defaultCfg.RemotePreScript, fmt.Sprintf("Remote script to run before starting transfer (default: %q)", defaultCfg.RemotePreScript))
	remoteFlags.String("remote_post_script", defaultCfg.RemotePostScript, fmt.Sprintf("Remote script to run after finishing transfer (default: %q)", defaultCfg.RemotePostScript))

	// Compression Options.
	compressionFlags.String("compress", defaultCfg.Compress, fmt.Sprintf("Compression type, options: %v", SupportedCompression))
	compressionFlags.Int("compress_level", defaultCfg.CompressLevel, fmt.Sprintf("Compression level for zstd (default: %d, balanced for CPU usage); ignored for lz4", defaultCfg.CompressLevel))

	// LVM Options.
	lvmFlags.Bool("skip_snapshot_creation", defaultCfg.SkipSnapshotCreation, "Skip automatic snapshot creation")
	lvmFlags.Bool("skip_disk_check", defaultCfg.SkipDiskCheck, "Skip disk space check before snapshot creation")
	lvmFlags.String("snapshot_size", defaultCfg.SnapshotSize, "Snapshot size as an absolute value (e.g. \"20G\") or as a percentage of the original volume (e.g. \"20%\")")
	lvmFlags.String("lvm_escalation", defaultCfg.LVMEscalation, "Command used to escalate privileges for LVM commands (default: \"sudo -n\")")
	lvmFlags.String("volume_group", defaultCfg.VolumeGroup, "Volume group name of the source LVM volume (default: \"vg0\")")

	pflag.CommandLine.AddFlagSet(generalFlags)
	pflag.CommandLine.AddFlagSet(sshFlags)
	pflag.CommandLine.AddFlagSet(remoteFlags)
	pflag.CommandLine.AddFlagSet(compressionFlags)
	pflag.CommandLine.AddFlagSet(lvmFlags)

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <snapshot|lvm device> <destination>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "General Options:\n")
		generalFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nSSH Options:\n")
		sshFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nRemote Options:\n")
		remoteFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCompression Options:\n")
		compressionFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nLVM Options:\n")
		lvmFlags.PrintDefaults()
	}

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("_", "."))
	v.SetEnvPrefix("LVMSYNC")

	pflag.Parse()
	if err := v.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}
	configFile := v.GetString("config")
	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file: %v", err)
		}
	}
	var conf Config
	if err := v.Unmarshal(&conf); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %v", err)
	}
	speedStr := v.GetString("speed")
	speedStr = strings.ReplaceAll(speedStr, " ", "")
	if speedVal, err := humanize.ParseBytes(speedStr); err == nil {
		conf.SpeedLimit = int(speedVal)
	} else {
		return nil, fmt.Errorf("invalid speed value %q: %v", speedStr, err)
	}
	return &conf, nil
}

func (c *Config) Validate() error {
	out, err := exec.Command("vgdisplay", c.VolumeGroup).CombinedOutput()
	if err != nil {
		return fmt.Errorf("volume group %q does not exist or is inaccessible: %v, output: %s", c.VolumeGroup, err, string(out))
	}
	if os.Geteuid() != 0 {
		parts := strings.Fields(c.LVMEscalation)
		if len(parts) == 0 {
			return fmt.Errorf("lvm escalation command is empty")
		}
		if _, err := exec.LookPath(parts[0]); err != nil {
			return fmt.Errorf("lvm escalation command %q not found: %v", parts[0], err)
		}
	}
	return nil
}
