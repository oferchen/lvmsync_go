package config

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/pierrec/lz4/v4"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"lvmsync_go/internal/compressiondetect"
	"lvmsync_go/internal/sizeparse"
)

type builder struct {
	v        *viper.Viper
	defaults *Config
}

func (b *builder) Build() (*Config, error) {
	registerKeyAliases(b.v)
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
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	return &conf, nil
}

func (b *builder) applyDefaults(conf *Config) error {
	if conf.Mode == "" {
		conf.Mode = b.defaults.Mode
	}
	if !isSet(b.v, "allow-insecure") {
		conf.AllowInsecure = b.defaults.AllowInsecure
	}
	if !isSet(b.v, "numa-pin") {
		conf.NumaPin = b.defaults.NumaPin
	}
	if !isSet(b.v, "numa-node") {
		conf.NumaNode = b.defaults.NumaNode
	}
	if conf.Transport == "" {
		conf.Transport = b.defaults.Transport
	}
	if conf.ManifestTimeout == 0 {
		conf.ManifestTimeout = b.defaults.ManifestTimeout
	}
	if conf.ManifestProgressInterval == 0 {
		conf.ManifestProgressInterval = b.defaults.ManifestProgressInterval
	}
	if !isSet(b.v, "manifest-allow-mounted") {
		conf.ManifestAllowMounted = b.defaults.ManifestAllowMounted
	}

	if conf.LVMEscalation == "" {
		conf.LVMEscalation = b.defaults.LVMEscalation
	}
	if conf.SSHKeepAliveInterval == 0 {
		conf.SSHKeepAliveInterval = b.defaults.SSHKeepAliveInterval
	}
	if conf.LVMTimeout == 0 {
		conf.LVMTimeout = b.defaults.LVMTimeout
	}
	if conf.TCPParallel == 0 {
		conf.TCPParallel = b.defaults.TCPParallel
	}
	if conf.CDCMin == 0 {
		conf.CDCMin = b.defaults.CDCMin
	}
	if conf.CDCAvg == 0 {
		conf.CDCAvg = b.defaults.CDCAvg
	}
	if conf.CDCMax == 0 {
		conf.CDCMax = b.defaults.CDCMax
	}
	if conf.ChunkSeed == 0 {
		conf.ChunkSeed = b.defaults.ChunkSeed
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

	cb, err := b.parseBytesOrFallback("checkpoint-bytes", b.defaults.CheckpointBytesRaw)
	if err != nil {
		return err
	}
	conf.CheckpointBytes = cb
	if conf.CheckpointBytesRaw == "" {
		conf.CheckpointBytesRaw = b.defaults.CheckpointBytesRaw
	}

	if conf.CompressConcurrency <= 0 {
		conf.CompressConcurrency = runtime.GOMAXPROCS(0)
	}
	if conf.CompressThreshold == 0 {
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

func (b *builder) applyThroughput(conf *Config) {
	if !isSet(b.v, "transport") {
		conf.Transport = "tcp+tls"
	}
	if !isSet(b.v, "parallel") {
		conf.Parallel = 8
	}
	if !isSet(b.v, "concurrency") {
		conf.Concurrency = 8
	}
	if !isSet(b.v, "dedup") {
		conf.DedupMode = "hybrid"
	}
	if !isSet(b.v, "block-size") {
		conf.BlockSize = 2 * 1024 * 1024
		conf.BlockSizeRaw = "2097152"
	}
	if !isSet(b.v, "cdc-min") {
		conf.CDCMin = 256 * 1024
	}
	if !isSet(b.v, "cdc-avg") {
		conf.CDCAvg = 2 * 1024 * 1024
	}
	if !isSet(b.v, "cdc-max") {
		conf.CDCMax = 8 * 1024 * 1024
	}
	if !isSet(b.v, "compress") {
		conf.Compress = Auto
	}
	if !isSet(b.v, "odirect") {
		conf.ODirect = true
	}
	if !isSet(b.v, "sync_interval") {
		conf.SyncInterval = "1GB"
		conf.SyncIntervalBytes = 1000000000
	}
	if !isSet(b.v, "checkpoint-interval") && conf.CheckpointInterval == 0 {
		conf.CheckpointInterval = 10 * time.Second
	}
}

func (b *builder) validateCompression(conf *Config) error {
	if err := validateCompressionSettings(conf); err != nil {
		return err
	}
	resolved := conf.Compress
	if resolved == "" || resolved == Auto {
		resolved = compressiondetect.DetectOptimalCompression()
	}
	if strings.Contains(resolved, ",") {
		return nil
	}
	switch resolved {
	case Zstd:
		conf.CompressLevel = conf.ZstdLevel
	case "lz4":
		if strings.ToLower(conf.LZ4Level) == "fast" {
			conf.CompressLevel = int(lz4.Fast)
		} else {
			conf.CompressLevel = int(lz4.Level9)
		}
	case "none":
		// no compression level for none
	default:
		return fmt.Errorf("unsupported compression algorithm: %s", conf.Compress)
	}
	return nil
}

func (b *builder) finalizeConfig(conf *Config) error {
	if strings.EqualFold(conf.ResumeState, "verify") {
		conf.ResumeVerify = true
		conf.ResumeState = defaultResumeState
	}
	algo := strings.ToLower(conf.ChecksumAlgorithm)
	switch algo {
	case "sha256", "blake3", Auto:
		conf.ChecksumAlgorithm = algo
	default:
		return fmt.Errorf("unsupported checksum algorithm: %s", conf.ChecksumAlgorithm)
	}

	if !conf.AllowInsecure && (conf.ClientCert != "" || conf.ClientKey != "" || conf.CACert != "") {
		if conf.ClientCert == "" || conf.ClientKey == "" {
			return fmt.Errorf("client-cert and client-key must be specified unless allow-insecure is set")
		}
		if _, err := os.Stat(conf.ClientCert); err != nil {
			return fmt.Errorf("client-cert: %w", err)
		}
		if _, err := os.Stat(conf.ClientKey); err != nil {
			return fmt.Errorf("client-key: %w", err)
		}
		if conf.CACert != "" {
			if _, err := os.Stat(conf.CACert); err != nil {
				return fmt.Errorf("ca-cert: %w", err)
			}
		}
	}
	return nil
}

func (b *builder) parseBytesOrFallback(key, fallback string) (int, error) {
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

func (b *builder) parseBlockSize() (int, string, error) {
	raw := strings.ReplaceAll(strings.TrimSpace(b.v.GetString("block-size")), " ", "")
	if raw == "" {
		raw = b.defaults.BlockSizeRaw
	}
	if strings.EqualFold(raw, Auto) {
		return 0, raw, nil
	}
	val, isPercent, err := sizeparse.Parse(raw)
	if err != nil || isPercent {
		return 0, "", fmt.Errorf("invalid block-size value %q: %w", raw, err)
	}
	u := uint64(val)
	if float64(u) != val || u > uint64(math.MaxInt) {
		return 0, "", fmt.Errorf("block-size value %q overflows int", raw)
	}
	return int(u), raw, nil
}

func buildViper(flagSets *FlagSets) (*viper.Viper, []string, bool, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.SetEnvPrefix("LVMSYNC")
	v.AutomaticEnv()
	var envErr error
	for _, fs := range flagSets.All() {
		if err := v.BindPFlags(fs); err != nil {
			return nil, nil, false, err
		}
		fs.VisitAll(func(f *pflag.Flag) {
			if envErr == nil {
				envErr = v.BindEnv(f.Name)
			}
			if strings.Contains(f.Name, "-") {
				alias := strings.ReplaceAll(f.Name, "-", "_")
				v.RegisterAlias(alias, f.Name)
			}
		})
	}
	if envErr != nil {
		return nil, nil, false, envErr
	}

	if err := bindTransportEnv(flagSets.Transport, v); err != nil {
		return nil, nil, false, err
	}
	if err := bindDedupEnv(flagSets.Dedup, v); err != nil {
		return nil, nil, false, err
	}
	if err := bindCompressionEnv(flagSets.Compression, v); err != nil {
		return nil, nil, false, err
	}
	if err := bindLVMEnv(flagSets.LVM, v); err != nil {
		return nil, nil, false, err
	}
	var warnings []string
	allowInsecureYAML := false
	if cfgFile := v.GetString("config"); cfgFile != "" {
		data, err := os.ReadFile(cfgFile)
		if err != nil {
			return nil, nil, false, fmt.Errorf("error reading config file %q: %w", cfgFile, err)
		}
		v.SetConfigFile(cfgFile)
		if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
			return nil, nil, false, fmt.Errorf("error reading config file %q: %w", cfgFile, err)
		}
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err == nil {
			if v, ok := raw["allow_insecure"].(bool); ok && v {
				allowInsecureYAML = true
			}
			if v, ok := raw["allow-insecure"].(bool); ok && v {
				allowInsecureYAML = true
			}
			valid := knownConfigKeys()
			for k := range raw {
				if _, ok := valid[k]; ok {
					continue
				}
				switch {
				case strings.Contains(k, "-"):
					if _, ok := valid[strings.ReplaceAll(k, "-", "_")]; ok {
						continue
					}
				case strings.Contains(k, "_"):
					if _, ok := valid[strings.ReplaceAll(k, "_", "-")]; ok {
						continue
					}
				}
				warnings = append(warnings, fmt.Sprintf("unknown configuration key %q", k))
			}
		}
	}
	return v, warnings, allowInsecureYAML, nil
}

func knownConfigKeys() map[string]struct{} {
	keys := make(map[string]struct{})
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("mapstructure")
		if tag != "" && tag != "-" {
			keys[tag] = struct{}{}
		}
	}
	return keys
}

func registerKeyAliases(v *viper.Viper) {
	for k := range knownConfigKeys() {
		switch {
		case strings.Contains(k, "-"):
			v.RegisterAlias(strings.ReplaceAll(k, "-", "_"), k)
		case strings.Contains(k, "_"):
			v.RegisterAlias(strings.ReplaceAll(k, "_", "-"), k)
		}
	}
}

func isSet(v *viper.Viper, key string) bool {
	if v.IsSet(key) {
		return true
	}
	if strings.Contains(key, "-") {
		return v.IsSet(strings.ReplaceAll(key, "-", "_"))
	}
	if strings.Contains(key, "_") {
		return v.IsSet(strings.ReplaceAll(key, "_", "-"))
	}
	return false
}
