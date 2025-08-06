package dedup

import (
	"encoding/hex"
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config holds runtime configuration for the deduplication pipeline.
type Config struct {
	MinChunkSize      int     `mapstructure:"min_chunk_size"`
	MaxChunkSize      int     `mapstructure:"max_chunk_size"`
	FalsePositiveRate float64 `mapstructure:"false_positive_rate"`
	RAMBytes          uint64  `mapstructure:"ram_bytes"`
	VolumeSize        uint64  `mapstructure:"volume_size"`
	HashKey           string  `mapstructure:"hash_key"`
}

// LoadConfig loads configuration from the provided YAML file, environment
// variables and CLI arguments. CLI arguments have highest precedence.
// Environment variables are expected to be prefixed with LVMSYNC_ and use
// uppercase names matching the struct fields.
func LoadConfig(path string, args []string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// defaults
	v.SetDefault("min_chunk_size", 4*1024)
	v.SetDefault("max_chunk_size", 1024*1024)
	v.SetDefault("false_positive_rate", 0.001)
	v.SetDefault("ram_bytes", uint64(1<<30))

	flags := pflag.NewFlagSet("dedup", pflag.ContinueOnError)
	flags.Int("min_chunk_size", 4*1024, "minimum chunk size in bytes")
	flags.Int("max_chunk_size", 1024*1024, "maximum chunk size in bytes")
	flags.Float64("false_positive_rate", 0.001, "Bloom filter false positive rate")
	flags.Uint64("ram_bytes", uint64(1<<30), "RAM budget for Bloom filter")
	flags.Uint64("volume_size", 0, "size of the volume being processed")
	flags.String("hash_key", "", "optional hex encoded key for BLAKE3")
	_ = flags.Parse(args)

	v.BindPFlags(flags)
	v.SetEnvPrefix("LVMSYNC")
	v.AutomaticEnv()

	if path != "" {
		if err := v.ReadInConfig(); err != nil {
			return Config{}, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// KeyBytes returns the optional hash key as raw bytes. An empty string
// results in nil.
func (c Config) KeyBytes() ([]byte, error) {
	if c.HashKey == "" {
		return nil, nil
	}
	b, err := hex.DecodeString(c.HashKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hash key: %w", err)
	}
	return b, nil
}
