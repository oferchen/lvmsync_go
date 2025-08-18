package config

import (
        "testing"
        "time"
)

func TestRetryDelayCLIOverridesEnvAndConfig(t *testing.T) {
        cfgPath := writeTempConfig(t, "retry_delay: 1s\n")
        rootFS, args := newFlagSet([]string{"--config", cfgPath, "--retry-delay", "3s"})
        t.Setenv("LVMSYNC_RETRY_DELAY", "2s")

        defaults, err := DefaultConfig()
        if err != nil {
                t.Fatalf("DefaultConfig: %v", err)
        }
        fs := NewFlagSets(defaults)
        registerFlags(fs, rootFS)
        if err := rootFS.Parse(args); err != nil {
                t.Fatalf("parse: %v", err)
        }
        v, _, err := buildViper(fs)
        if err != nil {
                t.Fatalf("buildViper: %v", err)
        }
        builder := &builder{v: v, defaults: defaults}
        conf, err := builder.Build()
        if err != nil {
                t.Fatalf("Build: %v", err)
        }
        if conf.RetryDelay != 3*time.Second {
                t.Fatalf("expected retry_delay 3s, got %v", conf.RetryDelay)
        }
}

func TestRetryDelayEnvOverridesConfig(t *testing.T) {
        cfgPath := writeTempConfig(t, "retry_delay: 1s\n")
        rootFS, args := newFlagSet([]string{"--config", cfgPath})
        t.Setenv("LVMSYNC_RETRY_DELAY", "2s")

        defaults, err := DefaultConfig()
        if err != nil {
                t.Fatalf("DefaultConfig: %v", err)
        }
        fs := NewFlagSets(defaults)
        registerFlags(fs, rootFS)
        if err := rootFS.Parse(args); err != nil {
                t.Fatalf("parse: %v", err)
        }
        v, _, err := buildViper(fs)
        if err != nil {
                t.Fatalf("buildViper: %v", err)
        }
        builder := &builder{v: v, defaults: defaults}
        conf, err := builder.Build()
        if err != nil {
                t.Fatalf("Build: %v", err)
        }
        if conf.RetryDelay != 2*time.Second {
                t.Fatalf("expected retry_delay 2s, got %v", conf.RetryDelay)
        }
}

