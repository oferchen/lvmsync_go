// config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	ConfigFile  string `mapstructure:"config"`
	ApplyMode   string `mapstructure:"apply"`
	StdoutMode  bool   `mapstructure:"stdout"`
	Parallel    int    `mapstructure:"parallel"`
	ZeroCopy    bool   `mapstructure:"zerocopy"`
	MaxRetries  int    `mapstructure:"max_retries"`
	ResumeState string `mapstructure:"resume"`

	SSHUser     string `mapstructure:"ssh_user"`
	SSHKeyPath  string `mapstructure:"ssh_key"`
	SSHPort     int    `mapstructure:"ssh_port"`
	KnownHosts  string `mapstructure:"known_hosts"`
	SSHVerify   bool   `mapstructure:"ssh_verify"`
	LVMSyncPath string `mapstructure:"lvmsync_path"`

	RemotePreScript  string `mapstructure:"remote_pre_script"`
	RemotePostScript string `mapstructure:"remote_post_script"`

	Compress   string `mapstructure:"compress"`
	Speed      string `mapstructure:"speed"`
	SpeedLimit int    `mapstructure:"-"`

	VerifyChecksum bool `mapstructure:"verify_checksum"`

	Verbose int `mapstructure:"verbose"`
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		ApplyMode:        "",
		StdoutMode:       false,
		Parallel:         4,
		ZeroCopy:         false,
		MaxRetries:       3,
		ResumeState:      "",
		SSHUser:          "root",
		SSHKeyPath:       "",
		SSHPort:          22,
		KnownHosts:       filepath.Join(homeDir, ".ssh", "known_hosts"),
		SSHVerify:        true,
		LVMSyncPath:      "lvmsync",
		RemotePreScript:  "",
		RemotePostScript: "",
		Compress:         "lz4",
		Speed:            humanize.Bytes(100 * 1024 * 1024), // default as "100MB"
		VerifyChecksum:   false,
		Verbose:          0,
	}
}

func LoadConfig() (*Config, error) {
	defaultCfg := DefaultConfig()
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("_", "."))
	v.SetEnvPrefix("LVMSYNC")

	pflag.String("config", "", "Path to config YAML file (default: none)")
	pflag.String("apply", defaultCfg.ApplyMode, fmt.Sprintf("Apply mode: read change dump from file ('-' for STDIN) and apply to destination device (default: %q)", defaultCfg.ApplyMode))
	pflag.Bool("stdout", defaultCfg.StdoutMode, fmt.Sprintf("Write change dump to STDOUT (default: %v)", defaultCfg.StdoutMode))
	pflag.Int("parallel", defaultCfg.Parallel, fmt.Sprintf("Number of concurrent workers (default: %d)", defaultCfg.Parallel))
	pflag.Bool("zerocopy", defaultCfg.ZeroCopy, fmt.Sprintf("Enable zero-copy transfers (default: %v)", defaultCfg.ZeroCopy))
	pflag.Int("max_retries", defaultCfg.MaxRetries, fmt.Sprintf("Maximum number of retries per block (default: %d)", defaultCfg.MaxRetries))
	pflag.String("resume", defaultCfg.ResumeState, fmt.Sprintf("Path to resume state file (default: %q)", defaultCfg.ResumeState))
	pflag.String("ssh_user", defaultCfg.SSHUser, fmt.Sprintf("SSH username (default: %q)", defaultCfg.SSHUser))
	pflag.String("ssh_key", defaultCfg.SSHKeyPath, fmt.Sprintf("Path to SSH private key or use agent (default: %q)", defaultCfg.SSHKeyPath))
	pflag.Int("ssh_port", defaultCfg.SSHPort, fmt.Sprintf("SSH port (default: %d)", defaultCfg.SSHPort))
	pflag.String("known_hosts", defaultCfg.KnownHosts, fmt.Sprintf("Path to known_hosts file (default: %q)", defaultCfg.KnownHosts))
	pflag.Bool("stricthostkeychecking", defaultCfg.SSHVerify, fmt.Sprintf("Enable SSH StrictHostKeyChecking (default: %v)", defaultCfg.SSHVerify))
	pflag.String("lvmsync_path", defaultCfg.LVMSyncPath, fmt.Sprintf("Remote command to run (default: %q)", defaultCfg.LVMSyncPath))
	pflag.String("remote_pre_script", defaultCfg.RemotePreScript, fmt.Sprintf("Remote script to run before starting transfer (default: %q)", defaultCfg.RemotePreScript))
	pflag.String("remote_post_script", defaultCfg.RemotePostScript, fmt.Sprintf("Remote script to run after finishing transfer (default: %q)", defaultCfg.RemotePostScript))
	pflag.String("compress", defaultCfg.Compress, fmt.Sprintf("Compression to use (default: %q)", defaultCfg.Compress))
	pflag.String("speed", defaultCfg.Speed, fmt.Sprintf("Speed limit (e.g. \"100MB\") (default: %s)", defaultCfg.Speed))
	pflag.CountP("verbose", "v", "Set verbosity level (e.g. -v, -vv, -vvv)")
	pflag.Bool("verify_checksum", defaultCfg.VerifyChecksum, fmt.Sprintf("Enable checksum verification (default: %v)", defaultCfg.VerifyChecksum))

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <snapshot device> <destination>\n", os.Args[0])
		pflag.PrintDefaults()
	}

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
