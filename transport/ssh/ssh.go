package ssh

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

const defaultDialTimeout = 5 * time.Second

// Transport implements the transport.Interface over SSH.
type Transport struct {
	serverConf *ssh.ServerConfig
	clientConf *ssh.ClientConfig
	hostSigner ssh.Signer
	logger     *zap.Logger
}

// New returns a new Transport using username/password authentication. Logger is optional; a no-op logger is used when nil.
func New(ctx context.Context, cfg transport.Config) (transport.Interface, error) {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.SSHUser == "" {
		return nil, fmt.Errorf("ssh user is required")
	}
	var (
		keySigner  ssh.Signer
		hostSigner ssh.Signer
		err        error
	)
	if cfg.SSHKeyPath != "" {
		keyBytes, err := os.ReadFile(cfg.SSHKeyPath)
		if err != nil {
			return nil, err
		}
		keySigner, err = ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, err
		}
	}

	if cfg.HostKeyPath != "" {
		hostBytes, err := os.ReadFile(cfg.HostKeyPath)
		if err != nil {
			return nil, err
		}
		hostSigner, err = ssh.ParsePrivateKey(hostBytes)
		if err != nil {
			return nil, err
		}
	} else if cfg.AllowInsecure {
		hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		hostSigner, err = ssh.NewSignerFromKey(hostKey)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("host key path required when allow_insecure is false")
	}

	serverConf := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == cfg.SSHUser && string(pass) == cfg.SSHPassword {
				return nil, nil
			}
			return nil, fmt.Errorf("authentication failed")
		},
	}
	serverConf.AddHostKey(hostSigner)
	if keySigner != nil {
		serverConf.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == cfg.SSHUser && bytes.Equal(key.Marshal(), keySigner.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("public key rejected")
		}
	}

	var hkc ssh.HostKeyCallback
	switch {
	case cfg.SSHKnownHosts != "":
		hkc, err = knownhosts.New(cfg.SSHKnownHosts)
		if err != nil {
			return nil, fmt.Errorf("knownhosts: %w", err)
		}
	case cfg.SSHHostKey != "":
		pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(cfg.SSHHostKey))
		if err != nil {
			return nil, fmt.Errorf("parse host key: %w", err)
		}
		hkc = ssh.FixedHostKey(pk)
	case cfg.AllowInsecure:
		cfg.Logger.Warn("allow_insecure_enabled", zap.String("transport", "ssh"), zap.String("security", "host_key_verification_disabled"), zap.String("usage", "development_only"))
		hkc = ssh.InsecureIgnoreHostKey()
	default:
		return nil, fmt.Errorf("known hosts or host key required")
	}

	var auths []ssh.AuthMethod
	if keySigner != nil {
		auths = append(auths, ssh.PublicKeys(keySigner))
	}
	if cfg.SSHUseAgent {
		if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
			agentCtx, cancel := context.WithTimeout(ctx, defaultDialTimeout)
			defer cancel()
			if signers, err := agentSigners(agentCtx, sock); err == nil && len(signers) > 0 {
				auths = append(auths, ssh.PublicKeys(signers...))
			}
		}
	}
	if cfg.SSHPassword != "" {
		auths = append(auths, ssh.Password(cfg.SSHPassword))
	}
	if len(auths) == 0 {
		return nil, fmt.Errorf("no authentication methods configured")
	}

	clientConf := &ssh.ClientConfig{
		User:            cfg.SSHUser,
		Auth:            auths,
		HostKeyCallback: hkc,
	}

	return &Transport{serverConf: serverConf, clientConf: clientConf, hostSigner: hostSigner, logger: cfg.Logger}, nil
}

func agentSigners(ctx context.Context, sock string) ([]ssh.Signer, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ag := agent.NewClient(conn)
	return ag.Signers()
}

func init() {
	transport.MustRegister("ssh", func(cfg transport.Config) (transport.Interface, error) {
		return New(context.Background(), cfg)
	})
}

func (t *Transport) Name() string { return "ssh" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	role := "client"
	t.logger.Info("dial_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	d := &net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", address)
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	dl, ok := ctx.Deadline()
	if !ok {
		dl = time.Now().Add(defaultDialTimeout)
	}
	if err := raw.SetDeadline(dl); err != nil {
		raw.Close()
		fields = append(fields, zap.Error(err))
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			raw.SetDeadline(time.Now())
		case <-done:
		}
	}()
	cc, chans, reqs, err := ssh.NewClientConn(raw, address, t.clientConf)
	close(done)
	fields = []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		raw.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		fields = append(fields, zap.Error(err))
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		raw.Close()
		cc.Close()
		fields = append(fields, zap.Error(err))
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	client := ssh.NewClient(cc, chans, reqs)
	ch, chReqs, err := client.OpenChannel("session", nil)
	fields = []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		client.Close()
		fields = append(fields, zap.Error(err))
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	go ssh.DiscardRequests(chReqs)
	t.logger.Info("dial_end", fields...)
	return &sshConn{netConn: raw, channel: ch, client: client, logger: t.logger}, nil
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	role := "server"
	t.logger.Info("listen_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	lc := net.ListenConfig{}
	tcpLn, err := lc.Listen(ctx, "tcp", address)
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		t.logger.Error("listen_end", fields...)
		return nil, err
	}
	ln := &sshListener{Listener: tcpLn, config: t.serverConf, logger: t.logger}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	t.logger.Info("listen_end", fields...)
	return ln, nil
}

func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (peer common.Handshake, err error) {
	roleStr := role.String()
	address := conn.RemoteAddr().String()
	t.logger.Info("negotiate_start",
		zap.String("address", address),
		zap.String("role", roleStr),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", roleStr),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		}
		if err != nil {
			fields = append(fields, zap.Error(err))
			t.logger.Error("negotiate_end", fields...)
		} else {
			fields = append(fields, transport.HandshakeFields(hs)...)
			t.logger.Info("negotiate_end", fields...)
		}
	}()
	hs.Version = common.ProtocolVersion
	if hs.Endianness == "" {
		hs.Endianness = common.NativeEndianness()
	}
	switch role {
	case transport.Client:
		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		if err = common.WriteHandshake(conn, hs); err != nil {
			clearDeadline(conn)
			return peer, err
		}
		clearDeadline(conn)

		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		peer, err = common.ReadHandshake(bufio.NewReader(conn))
		clearDeadline(conn)
		if err != nil {
			return peer, err
		}
		hs = common.MergeHandshake(hs, peer)
		if err := common.ValidateHandshake(hs, peer); err != nil {
			return peer, err
		}
		return peer, nil
	case transport.Server:
		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		peer, err = common.ReadHandshake(bufio.NewReader(conn))
		clearDeadline(conn)
		if err != nil {
			return peer, err
		}
		hs = common.MergeHandshake(hs, peer)
		if err := common.ValidateHandshake(hs, peer); err != nil {
			return peer, err
		}
		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		if err = common.WriteHandshake(conn, hs); err != nil {
			clearDeadline(conn)
			return peer, err
		}
		clearDeadline(conn)
		return peer, nil
	default:
		return peer, nil
	}
}

type sshConn struct {
	netConn net.Conn
	channel ssh.Channel
	client  *ssh.Client
	logger  *zap.Logger
}

func (s *sshConn) Read(b []byte) (int, error)  { return s.channel.Read(b) }
func (s *sshConn) Write(b []byte) (int, error) { return s.channel.Write(b) }
func (s *sshConn) Close() error {
	role := "client"
	address := s.netConn.RemoteAddr().String()
	s.logger.Info("close_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	err := s.client.Close()
	s.channel.Close()
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		s.logger.Error("close_end", fields...)
	} else {
		s.logger.Info("close_end", fields...)
	}
	return err
}
func (s *sshConn) LocalAddr() net.Addr                { return s.netConn.LocalAddr() }
func (s *sshConn) RemoteAddr() net.Addr               { return s.netConn.RemoteAddr() }
func (s *sshConn) SetDeadline(t time.Time) error      { return s.netConn.SetDeadline(t) }
func (s *sshConn) SetReadDeadline(t time.Time) error  { return s.netConn.SetReadDeadline(t) }
func (s *sshConn) SetWriteDeadline(t time.Time) error { return s.netConn.SetWriteDeadline(t) }

type sshListener struct {
	net.Listener
	config *ssh.ServerConfig
	logger *zap.Logger
}

func (l *sshListener) Accept() (net.Conn, error) {
	raw, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	sc, chans, reqs, err := ssh.NewServerConn(raw, l.config)
	if err != nil {
		raw.Close()
		return nil, err
	}
	go ssh.DiscardRequests(reqs)
	newChan, ok := <-chans
	if !ok {
		sc.Close()
		return nil, io.EOF
	}
	ch, chReqs, err := newChan.Accept()
	if err != nil {
		sc.Close()
		return nil, err
	}
	go ssh.DiscardRequests(chReqs)
	return &serverConn{sshConn: sc, netConn: raw, channel: ch, logger: l.logger}, nil
}

type serverConn struct {
	sshConn *ssh.ServerConn
	netConn net.Conn
	channel ssh.Channel
	logger  *zap.Logger
}

func (s *serverConn) Read(b []byte) (int, error)  { return s.channel.Read(b) }
func (s *serverConn) Write(b []byte) (int, error) { return s.channel.Write(b) }
func (s *serverConn) Close() error {
	role := "server"
	address := s.netConn.RemoteAddr().String()
	s.logger.Info("close_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	err := s.sshConn.Close()
	s.channel.Close()
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		s.logger.Error("close_end", fields...)
	} else {
		s.logger.Info("close_end", fields...)
	}
	return err
}
func (s *serverConn) LocalAddr() net.Addr                { return s.netConn.LocalAddr() }
func (s *serverConn) RemoteAddr() net.Addr               { return s.netConn.RemoteAddr() }
func (s *serverConn) SetDeadline(t time.Time) error      { return s.netConn.SetDeadline(t) }
func (s *serverConn) SetReadDeadline(t time.Time) error  { return s.netConn.SetReadDeadline(t) }
func (s *serverConn) SetWriteDeadline(t time.Time) error { return s.netConn.SetWriteDeadline(t) }

func setDeadline(ctx context.Context, conn net.Conn) error {
	if d, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(d)
	}
	return nil
}

func clearDeadline(conn net.Conn) {
	_ = conn.SetDeadline(time.Time{})
}
