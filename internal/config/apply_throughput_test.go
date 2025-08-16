package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestBuilderApplyThroughput(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	v := viper.New()
	v.Set("mode", "throughput")
	b := &builder{v: v, defaults: defaults}
	conf := &Config{Mode: "throughput"}
	b.applyThroughput(conf)
	if conf.Transport != "tcp+tls" {
		t.Fatalf("expected transport tcp+tls, got %s", conf.Transport)
	}
	if conf.Parallel != 8 {
		t.Fatalf("expected parallel 8, got %d", conf.Parallel)
	}
	if conf.DedupMode != "hybrid" {
		t.Fatalf("expected dedup hybrid, got %s", conf.DedupMode)
	}
}
