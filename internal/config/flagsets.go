package config

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const defaultResumeState = "resume.json"

// FlagSets groups the flag sets for different configuration areas.
type FlagSets struct {
	General     *pflag.FlagSet
	SSH         *pflag.FlagSet
	Remote      *pflag.FlagSet
	Dedup       *pflag.FlagSet
	Compression *pflag.FlagSet
	LVM         *pflag.FlagSet
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
		f.Transport,
		f.Manifest,
	}
}

func initGeneralFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("General Options", pflag.ExitOnError)
	fs.String("config", "", "Path to config YAML file")
	fs.Bool("stdout", cfg.StdoutMode, "Write change dump to STDOUT")
	fs.Bool("strict-config", cfg.StrictConfig, "Treat configuration warnings as errors")
	fs.Bool("yes-i-know", cfg.YesIKnow, "Confirm destructive write operations in non-interactive sessions")
	fs.Bool("dry-run", cfg.DryRun, "Print actions without executing")
  fs.Bool("probe-only", cfg.ProbeOnly, "Output size_bytes, kernel_uuid, gpt_uuid, mbr_signature, fs_uuid, major, minor, and manifest_epoch without writing")
	fs.Bool("plan", cfg.Plan, "Print configuration plan as JSON and exit")
	fs.Bool("force", cfg.Force, "Override safety checks for offline raw access or filesystem freeze")
	fs.Bool("force-offline", cfg.ForceOffline, "Allow direct device writes; prompts for double-confirm")
	fs.Bool("allow-overwrite", cfg.AllowOverwrite, "Allow overwriting existing data; requires --yes-i-know for non-interactive sessions")
	fs.Bool("enable-quic", cfg.EnableQUIC, "Enable QUIC transport")
	fs.Bool("check-partition", cfg.CheckPartition, "Verify partition signatures for source and destination")
	fs.Bool("discard", cfg.Discard, "Issue BLKDISCARD before writing blocks and verify discarded regions")
	fs.Bool("offline", cfg.Offline, "Assume source raw device is offline")
	fs.Bool("sanitize-env", cfg.SanitizeEnv, "Drop PATH, LANG, and unsafe variables before privilege escalation (enable with --sanitize-env)")
	fs.String("sparse", cfg.Sparse, "Sparse file handling: auto or never")
	fs.String("fs-freeze-command", cfg.FSFreezeCommand, "Freeze command (absolute path, validated for NUL bytes, allowed characters, and existence)")
	fs.String("fs-thaw-command", cfg.FSThawCommand, "Thaw command (absolute path, validated for NUL bytes, allowed characters, and existence)")
	fs.Duration("freeze-timeout", cfg.FreezeTimeout, "Timeout for filesystem freeze command")
	fs.Duration("thaw-timeout", cfg.ThawTimeout, "Timeout for filesystem thaw command")
	fs.String("source-type", cfg.SourceType, "Source device type (auto,file,raw,lvm)")
	fs.String("dest-type", cfg.DestType, "Destination device type (auto,file,raw,lvm)")
	fs.String("mode", cfg.Mode, "Preset mode: default or throughput")
	fs.Int("parallel", cfg.Parallel, "Number of concurrent workers")
	fs.Bool("zerocopy", cfg.ZeroCopy, "Enable zero-copy transfers")
	fs.Bool("odirect", cfg.ODirect, "Use O_DIRECT for device I/O when possible")
	fs.Bool("numa-pin", cfg.NumaPin, "Pin worker goroutines to device NUMA node")
	fs.Int("numa-node", cfg.NumaNode, "NUMA node to pin worker goroutines")
	fs.Int("max-retries", cfg.MaxRetries, "Maximum number of retries per block")
	fs.Duration("retry-delay", cfg.RetryDelay, "Initial delay between retries")
	fs.String("resume", cfg.ResumeState, "Path to resume state file")
	fs.Bool("verify-only", cfg.VerifyOnly, "Verify destination without writing data")
	fs.String("speed", cfg.Speed, "Transfer speed limit")
	fs.String("sync-interval", cfg.SyncInterval, "Bytes between fdatasync calls")
	fs.String("checkpoint-bytes", cfg.CheckpointBytesRaw, "Bytes between resume checkpoints")
	fs.Duration("checkpoint-interval", cfg.CheckpointInterval, "Duration between checkpoints")
	fs.String("block-size", cfg.BlockSizeRaw, "Block size for data transfer; specify 'auto' or 0 for automatic detection")
	fs.String("delta", cfg.Delta, "Delta algorithm (none, rsync) to precompute byte-level changes")
	fs.CountP("verbose", "v", "Verbosity level")
	fs.Bool("verify-checksum", cfg.VerifyChecksum, "Enable checksum verification")
	fs.String("verify", cfg.VerifyLevel, "Verification level: inline, post, or none")
	fs.String("digest", cfg.ChecksumAlgorithm, fmt.Sprintf("Digest algorithm: %v", SupportedChecksumAlgorithms))
	fs.Bool("progress", cfg.Progress, "Show progress during transfer")
	fs.String("output", cfg.Output, "Output format: text, json, or yaml")
	return fs
}

func initManifestFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Manifest Options", pflag.ExitOnError)
	fs.String("manifest-path", cfg.ManifestPath, "Path to manifest file")
	fs.Duration("manifest-timeout", cfg.ManifestTimeout, "Timeout for manifest rebuild (0 to disable)")
	fs.Duration("manifest-progress-interval", cfg.ManifestProgressInterval, "Interval between progress logs during manifest rebuild")
	fs.Bool("manifest-allow-mounted", cfg.ManifestAllowMounted, "Allow rebuilding when device is mounted read-write")
	return fs
}

func initSSHFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	fs.String("ssh-host", cfg.SSHHost, "SSH host")
	fs.String("ssh-user", cfg.SSHUser, "SSH username")
	fs.String("ssh-key", cfg.SSHKeyPath, "Path to SSH private key or use agent")
	fs.String("ssh-host-key-path", cfg.SSHHostKeyPath, "Path to SSH host private key")
	fs.Int("ssh-port", cfg.SSHPort, "SSH port")
	fs.Duration("ssh-timeout", cfg.SSHTimeout, "SSH connection timeout")
	fs.Duration("ssh-keepalive", cfg.SSHKeepAliveInterval, "SSH keepalive interval")
	fs.String("ssh-host-key", cfg.SSHHostKey, "Expected SSH host public key")
	fs.String("known-hosts", cfg.KnownHosts, "Path to known_hosts file")
	fs.Bool("strict-host-key-checking", cfg.StrictHostKeyCheck, "Require host keys to be present in known_hosts")
	return fs
}

func initRemoteFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	fs.String("lvmsync-path", cfg.LVMSyncPath, "Remote command to run")
	fs.String("remote-pre-script", cfg.RemotePreScript, "Remote script to run before transfer")
	fs.String("remote-post-script", cfg.RemotePostScript, "Remote script to run after transfer")
	return fs
}

func initDedupFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Deduplication Options", pflag.ExitOnError)
	fs.String("dedup", cfg.DedupMode, fmt.Sprintf("Deduplication mode: %v", SupportedDedupModes))
	fs.Int("cdc-min", cfg.CDCMin, "Minimum chunk size for CDC")
	fs.Int("cdc-avg", cfg.CDCAvg, "Average chunk size for CDC")
	fs.Int("cdc-max", cfg.CDCMax, "Maximum chunk size for CDC")
	fs.Uint64("chunk-seed", cfg.ChunkSeed, "Seed for chunking")
	fs.String("dedup-strategy", cfg.DedupStrategy, fmt.Sprintf("Deduplication strategy: %v", SupportedDedupStrategies))
	fs.String("dedup-state-file", cfg.DedupStateFile, "Path to deduplication state file")
	fs.Bool("intra-dedup", cfg.IntraDedup, "Enable intra-run deduplication")
	fs.Int("bloom-entries", cfg.BloomEntries, "Bloom filter entries")
	fs.Float64("bloom-fp-rate", cfg.BloomFpRate, "Bloom filter false positive rate")
	fs.Uint("bloom-mbits", cfg.BloomMBits, "Bloom filter M bits per entry")
	return fs
}

func initCompressionFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	fs.String("compress", cfg.Compress, fmt.Sprintf("Compression algorithm: %v", SupportedCompression))
	fs.Int("zstd-level", cfg.ZstdLevel, "Zstd compression level (1-5)")
	fs.String("lz4-level", cfg.LZ4Level, "LZ4 compression level: fast or hc")
	fs.Int("compress-concurrency", cfg.CompressConcurrency, "Compression concurrency")
	fs.Float64("compress-threshold", cfg.CompressThreshold, "Skip compression when estimated ratio exceeds this value")
	return fs
}

func initLVMFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
	fs.Bool("skip-snapshot-creation", cfg.SkipSnapshotCreation, "Skip snapshot creation")
	fs.Bool("skip-disk-check", cfg.SkipDiskCheck, "Skip disk space check")
	fs.String("snapshot-size", cfg.SnapshotSize, "Snapshot size (bytes or percentage)")
	fs.Float64("snapshot-max-usage", cfg.SnapshotMaxUsage, "Maximum allowed snapshot usage percent before aborting")
	fs.String("lvm-escalation", cfg.LVMEscalation, "Command to use for privilege escalation")
	fs.Duration("lvm-timeout", cfg.LVMTimeout, "Timeout for LVM commands and privilege checks")
	fs.Duration("sig-cache-ttl", cfg.SigCacheTTL, "TTL for LVM signature cache entries")
	fs.Int("sig-cache-max", cfg.SigCacheMax, "Maximum LVM signature cache entries")
	fs.String("volume-group", cfg.VolumeGroup, "LVM volume group")
	fs.String("target-volume-group", cfg.TargetVolumeGroup, "Target LVM volume group")
	fs.StringSlice("target-vgs", cfg.TargetVGCandidates, "Candidate target VGs for volume selection")

	fs.Bool("create-dest-lv", cfg.CreateDestLV, "Create destination logical volume when missing")

	return fs
}

func initTransportFlags(cfg *Config) *pflag.FlagSet {
	fs := pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
	fs.String("transport", cfg.Transport, "Transport modes (comma-separated)")
	fs.Int("concurrency", cfg.Concurrency, "Number of concurrent connections")
	fs.Int("tcp-port", cfg.TCPPort, "TCP port")
	fs.Int("tcp-parallel", cfg.TCPParallel, "Number of parallel TCP connections per worker")
	fs.Int("tcp-lowat", cfg.TCPNotSentLowAt, "TCP_NOTSENT_LOWAT threshold in bytes")
	fs.Bool("allow-insecure", cfg.AllowInsecure, "allow insecure connections (disable TLS and host key verification)")
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
