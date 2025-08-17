package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func TestFlagParsing(t *testing.T) {
	logger := zap.NewNop()
	var got Options
	r := NewRunnerWithDeps(func(ctx context.Context, opts Options, _ *zap.Logger) error {
		got = opts
		return nil
	})
	args := []string{
		"--listen", "tcp://:8080",
		"--listen", "unix:///tmp/sock",
		"--module", "mod1",
		"--module", "mod2",
		"--sudo-helper", "/bin/helper",
		"--once",
	}
	if err := r.Execute(args, logger); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := Options{
		Listen:     []string{"tcp://:8080", "unix:///tmp/sock"},
		Modules:    []string{"mod1", "mod2"},
		Once:       true,
		SudoHelper: "/bin/helper",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

type bindErrViper struct {
	*viper.Viper
}

func (b *bindErrViper) BindPFlags(_ *pflag.FlagSet) error { return errors.New("bind fail") }
func (b *bindErrViper) Underlying() *viper.Viper          { return b.Viper }

func TestNewCmdBindError(t *testing.T) {
	r := NewRunner()
	if _, err := r.NewCmd(zap.NewNop(), &bindErrViper{Viper: viper.New()}); err == nil || err.Error() != "bind fail" {
		t.Fatalf("expected bind fail, got %v", err)
	}
}
