package config

import "testing"

func TestFSFreezeThawCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "fs-freeze-command: /cfg/freeze\nfs-thaw-command: /cfg/thaw\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--fs-freeze-command", "/cli/freeze", "--fs-thaw-command", "/cli/thaw"})
	t.Setenv("LVMSYNC_FS_FREEZE_COMMAND", "/env/freeze")
	t.Setenv("LVMSYNC_FS_THAW_COMMAND", "/env/thaw")

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
	if conf.FSFreezeCommand != "/cli/freeze" {
		t.Fatalf("expected fs-freeze-command /cli/freeze, got %s", conf.FSFreezeCommand)
	}
	if conf.FSThawCommand != "/cli/thaw" {
		t.Fatalf("expected fs-thaw-command /cli/thaw, got %s", conf.FSThawCommand)
	}
}

func TestFSFreezeThawEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "fs-freeze-command: /cfg/freeze\nfs-thaw-command: /cfg/thaw\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_FS_FREEZE_COMMAND", "/env/freeze")
	t.Setenv("LVMSYNC_FS_THAW_COMMAND", "/env/thaw")

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
	if conf.FSFreezeCommand != "/env/freeze" {
		t.Fatalf("expected fs-freeze-command /env/freeze, got %s", conf.FSFreezeCommand)
	}
	if conf.FSThawCommand != "/env/thaw" {
		t.Fatalf("expected fs-thaw-command /env/thaw, got %s", conf.FSThawCommand)
	}
}

func TestFSFreezeCommandRejectsRelativePath(t *testing.T) {
	rootFS, args := newFlagSet([]string{"--fs-freeze-command", "freeze"})
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

	b := &builder{v: v, defaults: defaults}
	if _, err := b.Build(); err == nil {
		t.Fatalf("expected error for relative path")
	}
}

func TestFSThawCommandRejectsRelativePath(t *testing.T) {
	rootFS, args := newFlagSet([]string{"--fs-thaw-command", "thaw"})
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

	b := &builder{v: v, defaults: defaults}
	if _, err := b.Build(); err == nil {
		t.Fatalf("expected error for relative path")
	}
}
