package config

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/pflag"
)

func TestLoadConfigDefaults(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	flagSets := NewFlagSets(defaults)

	conf, _, err := LoadConfig(flagSets, defaults, fs, nil)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.LVMEscalation != defaults.LVMEscalation {
		t.Fatalf("LVMEscalation = %q, want %q", conf.LVMEscalation, defaults.LVMEscalation)
	}
	if conf.SSHKeepAliveInterval != defaults.SSHKeepAliveInterval {
		t.Fatalf("SSHKeepAliveInterval = %v, want %v", conf.SSHKeepAliveInterval, defaults.SSHKeepAliveInterval)
	}
	if conf.LVMTimeout != defaults.LVMTimeout {
		t.Fatalf("LVMTimeout = %v, want %v", conf.LVMTimeout, defaults.LVMTimeout)
	}
	if conf.GRPCDialTimeout != defaults.GRPCDialTimeout {
		t.Fatalf("GRPCDialTimeout = %v, want %v", conf.GRPCDialTimeout, defaults.GRPCDialTimeout)
	}
	if conf.GRPCSetupTimeout != defaults.GRPCSetupTimeout {
		t.Fatalf("GRPCSetupTimeout = %v, want %v", conf.GRPCSetupTimeout, defaults.GRPCSetupTimeout)
	}
	if conf.HeartbeatInterval != defaults.HeartbeatInterval {
		t.Fatalf("HeartbeatInterval = %v, want %v", conf.HeartbeatInterval, defaults.HeartbeatInterval)
	}
	if conf.HeartbeatSendTimeout != defaults.HeartbeatSendTimeout {
		t.Fatalf("HeartbeatSendTimeout = %v, want %v", conf.HeartbeatSendTimeout, defaults.HeartbeatSendTimeout)
	}
	if conf.TCPParallel != defaults.TCPParallel {
		t.Fatalf("TCPParallel = %d, want %d", conf.TCPParallel, defaults.TCPParallel)
	}
	if conf.CDCMin != defaults.CDCMin {
		t.Fatalf("CDCMin = %d, want %d", conf.CDCMin, defaults.CDCMin)
	}
	if conf.CDCAvg != defaults.CDCAvg {
		t.Fatalf("CDCAvg = %d, want %d", conf.CDCAvg, defaults.CDCAvg)
	}
	if conf.CDCMax != defaults.CDCMax {
		t.Fatalf("CDCMax = %d, want %d", conf.CDCMax, defaults.CDCMax)
	}
}
