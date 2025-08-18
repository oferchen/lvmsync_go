package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

// ConfigBuilder builds a Config from flags, environment variables, and YAML
// files. Precedence is flags > environment variables > YAML.
//
// The FlagSets field can be modified by callers before Build is invoked to
// control which options are available.
//
// Build returns the populated Config, remaining CLI arguments, any warnings for
// unknown YAML keys, and an error if parsing or validation fails.
//
// Example:
//
//  defaults, _ := config.DefaultConfig()
//  b := config.NewBuilder(defaults)
//  fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
//  cfg, args, warns, err := b.Build(fs, os.Args[1:])
//
// Callers should log warnings returned by Build.
//

type ConfigBuilder struct {
	defaults *Config
	// FlagSets groups CLI flag sets. Callers may replace individual flag sets
	// before Build for command-specific options.
	FlagSets *FlagSets
}

// NewBuilder constructs a ConfigBuilder using the provided defaults.
func NewBuilder(defaults *Config) *ConfigBuilder {
	return &ConfigBuilder{defaults: defaults, FlagSets: NewFlagSets(defaults)}
}

// Build parses the provided args using fs and returns the resulting Config.
func (b *ConfigBuilder) Build(fs *pflag.FlagSet, args []string) (*Config, []string, []string, error) {
	if fs == nil {
		fs = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
		fs.SetOutput(io.Discard)
	}
	registerFlags(b.FlagSets, fs)
	if err := fs.Parse(args); err != nil {
		return nil, nil, nil, err
	}

	resumeVerify := false
	if f := fs.Lookup("resume"); f != nil && f.Changed {
		if strings.EqualFold(f.Value.String(), "verify") {
			resumeVerify = true
			_ = f.Value.Set("")
			f.Changed = false
		}
	}
	v, warns, err := buildViper(b.FlagSets)
	if err != nil {
		return nil, nil, warns, err
	}
	vb := &builder{v: v, defaults: b.defaults}
	cfg, err := vb.Build()
	if err != nil {
		return nil, nil, warns, err
	}
	if resumeVerify {
		cfg.ResumeVerify = true
		cfg.ResumeState = defaultResumeState
	}
	if cfg.AllowInsecure {
		_, envSet := os.LookupEnv("LVMSYNC_ALLOW_INSECURE")
		flagSet := false
		if f := fs.Lookup("allow-insecure"); f != nil && f.Changed {
			flagSet = true
		}
		if !envSet && !flagSet {
			return nil, nil, warns, fmt.Errorf("allow_insecure requires --allow-insecure flag or LVMSYNC_ALLOW_INSECURE environment variable")
		}
	}
	return cfg, fs.Args(), warns, nil
}
