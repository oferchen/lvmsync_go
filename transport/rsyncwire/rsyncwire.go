package rsyncwire

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gokrazy/rsync"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
)

const defaultDialTimeout = 5 * time.Second

// Transport implements the rsync daemon handshake over plain TCP.
type Transport struct {
	logger *zap.Logger
}

// New constructs a Transport. Logger must be non-nil.
func New(cfg transport.Config) (transport.Interface, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if !cfg.AllowInsecure {
		return nil, fmt.Errorf("rsync transport requires AllowInsecure")
	}
	cfg.Logger.Warn(
		"plaintext_connection",
		zap.String("transport", "rsync"),
		zap.String("docs", "docs/transports.md"),
	)
	return &Transport{logger: cfg.Logger}, nil
}

func init() {
	transport.MustRegister("rsync", func(cfg transport.Config) (transport.Interface, error) {
		tr, err := New(cfg)
		if err != nil {
			return nil, err
		}
		if tr == nil {
			return nil, fmt.Errorf("rsync: nil transport")
		}
		return tr, nil
	})
}

func (t *Transport) Name() string { return "rsync" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	role := "client"
	t.logger.Info("dial_start", zap.String("address", address), zap.String("role", role), zap.Int64("duration_ms", 0))
	start := time.Now()
	d := &net.Dialer{}
	if dl, ok := ctx.Deadline(); ok {
		d.Deadline = dl
	} else {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultDialTimeout)
		defer cancel()
		d.Deadline, _ = ctx.Deadline()
	}
	conn, err := d.DialContext(ctx, "tcp", address)
	fields := []zap.Field{zap.String("address", address), zap.String("role", role), zap.Int64("duration_ms", time.Since(start).Milliseconds())}
	if err != nil {
		fields = append(fields, zap.Error(err))
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	t.logger.Info("dial_end", fields...)
	return conn, nil
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	role := "server"
	t.logger.Info("listen_start", zap.String("address", address), zap.String("role", role), zap.Int64("duration_ms", 0))
	start := time.Now()
	ln, err := net.Listen("tcp", address)
	fields := []zap.Field{zap.String("address", address), zap.String("role", role), zap.Int64("duration_ms", time.Since(start).Milliseconds())}
	if err != nil {
		fields = append(fields, zap.Error(err))
		t.logger.Error("listen_end", fields...)
		return nil, err
	}
	t.logger.Info("listen_end", fields...)
	return ln, nil
}

func setDeadline(ctx context.Context, conn net.Conn) error {
	if dl, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(dl)
	}
	return nil
}

func clearDeadline(conn net.Conn) {
	_ = conn.SetDeadline(time.Time{})
}

// Negotiate performs a minimal rsync daemon handshake.
func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (common.Handshake, error) {
	roleStr := role.String()
	addr := conn.RemoteAddr().String()
	t.logger.Info("negotiate_start", zap.String("address", addr), zap.String("role", roleStr), zap.Int64("duration_ms", 0))
	start := time.Now()
	var err error
	switch role {
	case transport.Client:
		if err = setDeadline(ctx, conn); err != nil {
			break
		}
		rd := bufio.NewReader(conn)
		line, e := rd.ReadString('\n')
		if e != nil {
			err = e
			break
		}
		if !strings.HasPrefix(line, "@RSYNCD:") {
			err = fmt.Errorf("unexpected greeting: %q", strings.TrimSpace(line))
			break
		}
		if _, e = fmt.Fprintf(conn, "@RSYNCD: %d\n", rsync.ProtocolVersion); e != nil {
			err = e
			break
		}
		if _, e = fmt.Fprint(conn, "\n"); e != nil {
			err = e
			break
		}
		if _, e = rd.ReadString('\n'); e != nil {
			err = e
			break
		}
		clearDeadline(conn)
	case transport.Server:
		if err = setDeadline(ctx, conn); err != nil {
			break
		}
		if _, err = fmt.Fprintf(conn, "@RSYNCD: %d\n", rsync.ProtocolVersion); err != nil {
			break
		}
		rd := bufio.NewReader(conn)
		line, e := rd.ReadString('\n')
		if e != nil {
			err = e
			break
		}
		if !strings.HasPrefix(line, "@RSYNCD:") {
			err = fmt.Errorf("unexpected greeting: %q", strings.TrimSpace(line))
			break
		}
		if _, e = rd.ReadString('\n'); e != nil {
			err = e
			break
		}
		if _, err = fmt.Fprint(conn, "@RSYNCD: EXIT\n"); err != nil {
			break
		}
		clearDeadline(conn)
	default:
		err = fmt.Errorf("invalid role %v", role)
	}
	fields := []zap.Field{zap.String("address", addr), zap.String("role", roleStr), zap.Int64("duration_ms", time.Since(start).Milliseconds())}
	if err != nil {
		fields = append(fields, zap.Error(err))
		t.logger.Error("negotiate_end", fields...)
	} else {
		t.logger.Info("negotiate_end", fields...)
	}
	return hs, err
}
