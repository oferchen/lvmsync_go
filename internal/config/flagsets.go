package config

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	Manifest    *pflag.FlagSet
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
		Manifest:    initManifestFlags(cfg),
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
		f.Manifest,
	}
}

func initGeneralFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("General Options", pflag.ExitOnError)
	fs.String("config", "", "Path to config YAML file")
	fs.String("apply", cfg.ApplyMode, "Apply mode: read change dump from file ('-' for STDIN) and apply to destination device")
	fs.Bool("stdout", cfg.StdoutMode, "Write change dump to STDOUT")
	fs.Bool("dry-run", cfg.DryRun, "Print actions without executing")
	fs.Bool("force", cfg.Force, "Override safety checks and proceed on mounted destination")
	fs.Bool("discard", cfg.Discard, "Issue BLKDISCARD before writing blocks")
	fs.Bool("offline", cfg.Offline, "Assume source raw device is offline")
	fs.String("fs-freeze-command", cfg.FSFreezeCommand, "Command to freeze filesystem before reading raw source")
	fs.String("fs-thaw-command", cfg.FSThawCommand, "Command to thaw filesystem after reading raw source")
	fs.Duration("freeze-timeout", cfg.FreezeTimeout, "Timeout for filesystem freeze command")
	fs.Duration("thaw-timeout", cfg.ThawTimeout, "Timeout for filesystem thaw command")
	fs.String("source-type", cfg.SourceType, "Source device type (auto,file,raw,lvm)")
	fs.String("dest-type", cfg.DestType, "Destination device type (auto,file,raw,lvm)")
	fs.String("mode", cfg.Mode, "Preset mode: default or throughput")
	fs.Int("parallel", cfg.Parallel, "Number of concurrent workers")
	fs.Bool("zerocopy", cfg.ZeroCopy, "Enable zero-copy transfers")
	fs.Bool("odirect", cfg.ODirect, "Use O_DIRECT for device I/O when possible")
	fs.Bool("numa_pin", cfg.NumaPin, "Pin worker goroutines to device NUMA node")
	fs.Int("max_retries", cfg.MaxRetries, "Maximum number of retries per block")
	fs.String("resume", cfg.ResumeState, "Path to resume state file")
	fs.String("speed", cfg.Speed, "Transfer speed limit")
	fs.String("sync-interval", cfg.SyncInterval, "Bytes between fdatasync calls")
	fs.String("checkpoint_bytes", cfg.CheckpointBytesRaw, "Bytes between resume checkpoints")
	fs.Duration("checkpoint_interval", cfg.CheckpointInterval, "Duration between checkpoints")
	fs.String("block_size", cfg.BlockSizeRaw, "Block size for data transfer; specify 'auto' or 0 for automatic detection")
	fs.CountP("verbose", "v", "Verbosity level")
	fs.Bool("verify_checksum", cfg.VerifyChecksum, "Enable checksum verification")
	fs.String("verify", cfg.VerifyLevel, "Verification level: full, sampled, or none")
	fs.String("digest", cfg.ChecksumAlgorithm, fmt.Sprintf("Digest algorithm: %v", SupportedChecksumAlgorithms))
	fs.Bool("progress", cfg.Progress, "Show progress during transfer")
	return fs
}

func initManifestFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Manifest Options", pflag.ExitOnError)
	fs.String("manifest_path", cfg.ManifestPath, "Path to manifest file")
	fs.Duration("manifest_timeout", cfg.ManifestTimeout, "Timeout for manifest rebuild (0 to disable)")
	fs.Duration("manifest_progress_interval", cfg.ManifestProgressInterval, "Interval between progress logs during manifest rebuild")
	fs.Bool("manifest_allow_mounted", cfg.ManifestAllowMounted, "Allow rebuilding when device is mounted read-write")
	return fs
}

func initSSHFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	fs.String("ssh_host", cfg.SSHHost, "SSH host")
	fs.String("ssh_user", cfg.SSHUser, "SSH username")
	fs.String("ssh_key", cfg.SSHKeyPath, "Path to SSH private key or use agent")
	fs.String("ssh_host_key_path", cfg.SSHHostKeyPath, "Path to SSH host private key")
	fs.Int("ssh_port", cfg.SSHPort, "SSH port")
	fs.Duration("ssh_timeout", cfg.SSHTimeout, "SSH connection timeout")
	fs.Duration("ssh_keepalive", cfg.SSHKeepAliveInterval, "SSH keepalive interval")
	fs.String("ssh_host_key", cfg.SSHHostKey, "Expected SSH host public key")
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
	fs.Int("cdc-min", cfg.CDCMin, "Minimum chunk size for CDC")
	fs.Int("cdc-avg", cfg.CDCAvg, "Average chunk size for CDC")
	fs.Int("cdc-max", cfg.CDCMax, "Maximum chunk size for CDC")
	fs.Uint64("chunk-seed", cfg.ChunkSeed, "Seed for chunking")
	fs.String("dedup_strategy", cfg.DedupStrategy, fmt.Sprintf("Deduplication strategy: %v", SupportedDedupStrategies))
	fs.String("dedup_state_file", cfg.DedupStateFile, "Path to deduplication state file")
	fs.Bool("intra-dedup", cfg.IntraDedup, "Enable intra-run deduplication")
	fs.Int("bloom_entries", cfg.BloomEntries, "Bloom filter entries")
	fs.Float64("bloom_fp_rate", cfg.BloomFpRate, "Bloom filter false positive rate")
	fs.Uint("bloom_mbits", cfg.BloomMBits, "Bloom filter M bits per entry")
	return fs
}

func initCompressionFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	fs.String("compress", cfg.Compress, fmt.Sprintf("Compression algorithm: %v", SupportedCompression))
	fs.Int("zstd-level", cfg.ZstdLevel, "Zstd compression level (1-5)")
	fs.String("lz4-level", cfg.LZ4Level, "LZ4 compression level: fast or hc")
	fs.Int("compress_concurrency", cfg.CompressConcurrency, "Compression concurrency")
	fs.Float64("compress-threshold", cfg.CompressThreshold, "Skip compression when estimated ratio exceeds this value")
	return fs
}

func initLVMFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
	fs.Bool("skip_snapshot_creation", cfg.SkipSnapshotCreation, "Skip snapshot creation")
	fs.Bool("skip_disk_check", cfg.SkipDiskCheck, "Skip disk space check")
	fs.String("snapshot_size", cfg.SnapshotSize, "Snapshot size (bytes or percentage)")
	fs.String("lvm-escalation", cfg.LVMEscalation, "Command to use for privilege escalation")
	fs.Duration("lvm_timeout", cfg.LVMTimeout, "Timeout for LVM commands")
	fs.Duration("sig-cache-ttl", cfg.SigCacheTTL, "TTL for LVM signature cache entries")
	fs.Int("sig-cache-max", cfg.SigCacheMax, "Maximum LVM signature cache entries")
	fs.String("volume_group", cfg.VolumeGroup, "LVM volume group")
	fs.String("target_volume_group", cfg.TargetVolumeGroup, "Target LVM volume group")
	fs.StringSlice("target_vgs", cfg.TargetVGCandidates, "Candidate target VGs for volume selection")
	return fs
}

func initGRPCFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("gRPC Options", pflag.ExitOnError)
	fs.Int("grpc_port", cfg.GRPCPort, "gRPC server port")
	fs.String("grpc_listen", cfg.GRPCListen, "gRPC listen address")
	fs.String("grpc_connect", cfg.GRPCConnect, "gRPC connect address")
	fs.Duration("grpc_dial_timeout", cfg.GRPCDialTimeout, "gRPC dial timeout")
	fs.Duration("grpc_setup_timeout", cfg.GRPCSetupTimeout, "gRPC setup timeout")
	fs.Duration("grpc_heartbeat_interval", cfg.HeartbeatInterval, "gRPC heartbeat interval")
	fs.Duration("grpc_heartbeat_send_timeout", cfg.HeartbeatSendTimeout, "gRPC heartbeat send timeout")
	fs.String("tls_cert", cfg.TLSCert, "Path to TLS certificate")
	fs.String("tls_key", cfg.TLSKey, "Path to TLS private key")
	fs.String("ca_cert", cfg.CACert, "Path to CA certificate")
	fs.Bool("allow_insecure", cfg.AllowInsecure, "Allow insecure connections")
	return fs
}

func initTransportFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
	fs.String("transport", cfg.Transport, "Transport modes (comma-separated)")
	fs.Int("concurrency", cfg.Concurrency, "Number of concurrent connections")
	fs.Int("tcp_port", cfg.TCPPort, "TCP port")
	fs.Int("tcp_parallel", cfg.TCPParallel, "Number of parallel TCP connections per worker")
	fs.Int("tcp_lowat", cfg.TCPNotSentLowAt, "TCP_NOTSENT_LOWAT threshold in bytes")
	return fs
}

func registerFlags(flagSets *FlagSets, rootFS *pflag.FlagSet) {
	for _, fs := range flagSets.All() {
		rootFS.AddFlagSet(fs)
	}
	rootFS.SortFlags = false
	rootFS.Usage = func() {
		for _, fs := range flagSets.All() {
			fmt.Fprintf(rootFS.Output(), "%s:\n", fs.Name())
			fs.SetOutput(rootFS.Output())
			fs.PrintDefaults()
			fmt.Fprintln(rootFS.Output())
		}
	}
}

// bindTransportEnv binds transport flag names to environment variables with the
// LVMSYNC_TRANSPORT_ prefix.
func bindTransportEnv(fs *pflag.FlagSet, v *viper.Viper) error {
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		env := "LVMSYNC_TRANSPORT_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if e := v.BindEnv(f.Name, env); e != nil {
			err = e
		}
	})
	return err
}

// bindDedupEnv binds dedup flag names to environment variables with the
// LVMSYNC_DEDUP_ prefix.
func bindDedupEnv(fs *pflag.FlagSet, v *viper.Viper) error {
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		name := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		name = strings.TrimPrefix(name, "DEDUP_")
		env := "LVMSYNC_DEDUP"
		if name != "" {
			env += "_" + name
		}
		if e := v.BindEnv(f.Name, env); e != nil {
			err = e
		}
	})
	return err
}

// bindCompressionEnv binds compression flag names to environment variables with the
// LVMSYNC_COMPRESSION_ prefix.
func bindCompressionEnv(fs *pflag.FlagSet, v *viper.Viper) error {
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		env := "LVMSYNC_COMPRESSION_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if e := v.BindEnv(f.Name, env); e != nil {
			err = e
		}
	})
	return err
}

// bindLVMEnv binds LVM flag names to environment variables with the
// LVMSYNC_LVM_ prefix.
func bindLVMEnv(fs *pflag.FlagSet, v *viper.Viper) error {
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		name := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		name = strings.TrimPrefix(name, "LVM_")
		env := "LVMSYNC_LVM"
		if name != "" {
			env += "_" + name
		}
		if e := v.BindEnv(f.Name, env); e != nil {
			err = e
		}
	})
	return err
}

// bindGRPCEnv binds gRPC flag names to environment variables with the
// LVMSYNC_GRPC_ prefix.
func bindGRPCEnv(fs *pflag.FlagSet, v *viper.Viper) error {
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		name := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		name = strings.TrimPrefix(name, "GRPC_")
		env := "LVMSYNC_GRPC"
		if name != "" {
			env += "_" + name
		}
		if e := v.BindEnv(f.Name, env); e != nil {
			err = e
		}
	})
	return err
}
