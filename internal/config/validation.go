package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kballard/go-shellquote"

	digest "lvmsync_go/internal/digest"
	"lvmsync_go/lvm"
)

// Validate verifies configuration values using the real OS euid.
func (c *Config) Validate() error { return c.ValidateWith(os.Geteuid) }

// ValidateWith verifies configuration values using the provided geteuid function.
func (c *Config) ValidateWith(geteuid func() int) error {
	if c.Mode != "default" && c.Mode != "throughput" {
		return fmt.Errorf("invalid mode %q: must be \"default\" or \"throughput\"", c.Mode)
	}
	if strings.ToLower(c.ChecksumAlgorithm) == "" || strings.ToLower(c.ChecksumAlgorithm) == Auto {
		c.ChecksumAlgorithm = digest.Select()
	}
	if c.SourceType != "" && c.SourceType != "auto" && c.SourceType != "file" && c.SourceType != "raw" && c.SourceType != "lvm" {
		return fmt.Errorf("invalid source type %q", c.SourceType)
	}
	if c.DestType != "" && c.DestType != "auto" && c.DestType != "file" && c.DestType != "raw" && c.DestType != "lvm" {
		return fmt.Errorf("invalid dest type %q", c.DestType)
	}
	if c.Output != "" && c.Output != "text" && c.Output != "json" {
		return fmt.Errorf("invalid output %q: must be \"text\" or \"json\"", c.Output)
	}
	if c.FSFreezeCommand != "" {
		parts, err := shellquote.Split(c.FSFreezeCommand)
		if err != nil {
			return fmt.Errorf("invalid fs-freeze-command: %w", err)
		}
		if len(parts) == 0 || !filepath.IsAbs(parts[0]) {
			return fmt.Errorf("fs-freeze-command path %q must be absolute", c.FSFreezeCommand)
		}
	}
	if c.FSThawCommand != "" {
		parts, err := shellquote.Split(c.FSThawCommand)
		if err != nil {
			return fmt.Errorf("invalid fs-thaw-command: %w", err)
		}
		if len(parts) == 0 || !filepath.IsAbs(parts[0]) {
			return fmt.Errorf("fs-thaw-command path %q must be absolute", c.FSThawCommand)
		}
	}
	if c.SSHKeepAliveInterval <= 0 {
		return fmt.Errorf("ssh keepalive interval must be > 0")
	}
	if c.TCPParallel < 1 || c.TCPParallel > 4 {
		return fmt.Errorf("tcp_parallel must be between 1 and 4")
	}
	if c.TCPNotSentLowAt < 0 {
		return fmt.Errorf("tcp_lowat must be >= 0")
	}
	if c.CDCMin <= 0 || c.CDCAvg <= 0 || c.CDCMax <= 0 {
		return fmt.Errorf("cdc sizes must be > 0")
	}
	if !(c.CDCMin <= c.CDCAvg && c.CDCAvg <= c.CDCMax) {
		return fmt.Errorf("invalid cdc ordering: min %d avg %d max %d", c.CDCMin, c.CDCAvg, c.CDCMax)
	}
	if c.VolumeGroup != "" {
		ctx, cancel := context.WithTimeout(context.Background(), c.LVMTimeout)
		defer cancel()
		if _, err := lvm.GetVolumeGroupFreeSpace(ctx, c.VolumeGroup); err != nil {
			return fmt.Errorf("volume group %q does not exist or is inaccessible: %w", c.VolumeGroup, err)
		}
	}
	if c.TargetVolumeGroup != "" {
		ctx, cancel := context.WithTimeout(context.Background(), c.LVMTimeout)
		defer cancel()
		if _, err := lvm.GetVolumeGroupFreeSpace(ctx, c.TargetVolumeGroup); err != nil {
			return fmt.Errorf("target volume group %q does not exist or is inaccessible: %w", c.TargetVolumeGroup, err)
		}
	}
	if geteuid() != 0 {
		parts, err := lvm.ParseEscalation(c.LVMEscalation)
		if err != nil {
			return err
		}
		if _, err := findInPath(parts[0]); err != nil {
			return fmt.Errorf("lvm escalation command %q not found: %w", parts[0], err)
		}
		if parts[0] == "sudo" {
			hasN := false
			for _, p := range parts[1:] {
				if p == "-n" {
					hasN = true
					break
				}
			}
			if !hasN {
				return fmt.Errorf("lvm escalation must use 'sudo -n'")
			}
		}
	}
	if err := validateCompressionSettings(c); err != nil {
		return err
	}
	return nil
}

func validateCompressionSettings(c *Config) error {
	if strings.TrimSpace(c.Compress) != "" {
		algos := strings.Split(c.Compress, ",")
		for _, a := range algos {
			a = strings.TrimSpace(strings.ToLower(a))
			if a == "" {
				continue
			}
			valid := false
			for _, supported := range SupportedCompression {
				if a == supported {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("unsupported compression algorithm: %s", a)
			}
		}
	}
	if c.ZstdLevel != 0 && (c.ZstdLevel < 1 || c.ZstdLevel > 5) {
		return fmt.Errorf("invalid zstd compression level: %d", c.ZstdLevel)
	}
	lz4 := strings.ToLower(c.LZ4Level)
	if lz4 != "" && lz4 != "fast" && lz4 != "hc" {
		return fmt.Errorf("invalid lz4 compression level: %s", c.LZ4Level)
	}
	if c.CompressThreshold <= 0 || c.CompressThreshold > 1 {
		return fmt.Errorf("invalid compress threshold: %f", c.CompressThreshold)
	}
	return nil
}

func findInPath(name string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		info, err := os.Stat(name)
		if err != nil {
			return "", err
		}
		if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
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
		if !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return full, nil
		}
	}
	return "", fmt.Errorf("executable %q not found in $PATH", name)
}
