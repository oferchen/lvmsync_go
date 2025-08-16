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

func (b *builder) applyDefaults(conf *Config) error {
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
	if conf.ManifestTimeout == 0 {
		conf.ManifestTimeout = b.defaults.ManifestTimeout
	}
	if conf.ManifestProgressInterval == 0 {
		conf.ManifestProgressInterval = b.defaults.ManifestProgressInterval
	}
	if !b.v.IsSet("manifest_allow_mounted") {
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
	if conf.GRPCDialTimeout == 0 {
		conf.GRPCDialTimeout = b.defaults.GRPCDialTimeout
	}
	if conf.GRPCSetupTimeout == 0 {
		conf.GRPCSetupTimeout = b.defaults.GRPCSetupTimeout
	}
	if conf.HeartbeatInterval == 0 {
		conf.HeartbeatInterval = b.defaults.HeartbeatInterval
	}
	if conf.HeartbeatSendTimeout == 0 {
		conf.HeartbeatSendTimeout = b.defaults.HeartbeatSendTimeout
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

	cb, err := b.parseBytesOrFallback("checkpoint_bytes", b.defaults.CheckpointBytesRaw)
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

func (b *builder) applyThroughput(conf *Config) {
	if !b.v.IsSet("transport") {
		conf.Transport = "tcp+tls"
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
}

func (b *builder) validateCompression(conf *Config) error {
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

func (b *builder) finalizeConfig(conf *Config) error {
	algo := strings.ToLower(conf.ChecksumAlgorithm)
	switch algo {
	case "sha256", "blake3", "blake3-512", Auto:
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

func buildViper(flagSets *FlagSets) (*viper.Viper, []string, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.SetEnvPrefix("LVMSYNC")
	v.AutomaticEnv()
	keys := []string{
		"source-type",
		"dest-type",
	}
	for _, k := range keys {
		if err := v.BindEnv(k); err != nil {
			return nil, nil, err
		}
	}
	for _, fs := range flagSets.All() {
		if err := v.BindPFlags(fs); err != nil {
			return nil, nil, err
		}
	}
	v.RegisterAlias("cdc_min", "cdc-min")
	v.RegisterAlias("cdc_avg", "cdc-avg")
	v.RegisterAlias("cdc_max", "cdc-max")
	v.RegisterAlias("chunk_seed", "chunk-seed")
	v.RegisterAlias("zstd_level", "zstd-level")
	v.RegisterAlias("lz4_level", "lz4-level")
	v.RegisterAlias("compress_threshold", "compress-threshold")
	v.RegisterAlias("lvm_escalation", "lvm-escalation")
	v.RegisterAlias("checksum_algorithm", "digest")
	if err := bindTransportEnv(flagSets.Transport, v); err != nil {
		return nil, nil, err
	}
	if err := bindDedupEnv(flagSets.Dedup, v); err != nil {
		return nil, nil, err
	}
	if err := bindCompressionEnv(flagSets.Compression, v); err != nil {
		return nil, nil, err
	}
	if err := bindLVMEnv(flagSets.LVM, v); err != nil {
		return nil, nil, err
	}
	if err := bindGRPCEnv(flagSets.GRPC, v); err != nil {
		return nil, nil, err
	}
	var warnings []string
	if cfgFile := v.GetString("config"); cfgFile != "" {
		data, err := os.ReadFile(cfgFile)
		if err != nil {
			return nil, nil, fmt.Errorf("error reading config file %q: %w", cfgFile, err)
		}
		v.SetConfigFile(cfgFile)
		if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
			return nil, nil, fmt.Errorf("error reading config file %q: %w", cfgFile, err)
		}
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err == nil {
			valid := knownConfigKeys()
			for k := range raw {
				if _, ok := valid[k]; !ok {
					warnings = append(warnings, fmt.Sprintf("unknown configuration key %q", k))
				}
			}
		}
	}
	return v, warnings, nil
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
