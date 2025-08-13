package config

import (
	"os"

	"github.com/spf13/pflag"
)

// LoadConfig parses flags, builds a Viper instance, and returns the resulting configuration.
func LoadConfig(flagSets *FlagSets, defaults *Config, fs *pflag.FlagSet, args []string) (*Config, []string, error) {
	registerFlags(flagSets, fs)
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	v, err := buildViper(flagSets)
	if err != nil {
		return nil, nil, err
	}

	builder := &Builder{
		v:        v,
		defaults: defaults,
	}
	conf, err := builder.Build()
	if err != nil {
		return nil, nil, err
	}

	return conf, fs.Args(), nil
}

// Validate verifies configuration values using the real OS euid.
func (c *Config) Validate() error { return c.ValidateWith(os.Geteuid) }
