package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	lvmagent "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// mockAgent implements lvmagent.Agent for testing.
type mockAgent struct {
	lock      func(ctx context.Context, volume, requester string) error
	unlock    func(ctx context.Context, volume, requester string) error
	getMeta   func(ctx context.Context, volume string) (lvmagent.VolumeMetadata, error)
	sendMeta  func(ctx context.Context, md lvmagent.VolumeMetadata) error
	startSess func(ctx context.Context, volume, requester string) error
	finalize  func(ctx context.Context, volume, requester string) error
	status    func(ctx context.Context, volume, requester string) (string, error)

	sendBitmap    func(ctx context.Context, sessionID string, bitmap []byte) error
	sendManifest  func(ctx context.Context, sessionID string, manifest []byte) error
	finalizeSess  func(ctx context.Context, sessionID string) error
	ack           func(ctx context.Context, ack *proto.Ack) (*proto.Ack, error)
	probe         func(ctx context.Context, volume string) error
	cancel        func(ctx context.Context, sessionID string) error
	progress      func(ctx context.Context, sessionID string) (<-chan *proto.Progress, error)
	buildManifest func(ctx context.Context, sessionID string) error
	verify        func(ctx context.Context, sessionID string) error
}

func (m *mockAgent) Lock(ctx context.Context, volume, requester string) error {
	if m.lock != nil {
		return m.lock(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) Unlock(ctx context.Context, volume, requester string) error {
	if m.unlock != nil {
		return m.unlock(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) GetMetadata(ctx context.Context, volume string) (lvmagent.VolumeMetadata, error) {
	if m.getMeta != nil {
		return m.getMeta(ctx, volume)
	}
	return lvmagent.VolumeMetadata{}, nil
}
func (m *mockAgent) SendMetadata(ctx context.Context, md lvmagent.VolumeMetadata) error {
	if m.sendMeta != nil {
		return m.sendMeta(ctx, md)
	}
	return nil
}
func (m *mockAgent) StartTransferSession(ctx context.Context, volume, requester string) error {
	if m.startSess != nil {
		return m.startSess(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) FinalizeSync(ctx context.Context, volume, requester string) error {
	if m.finalize != nil {
		return m.finalize(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) GetStatus(ctx context.Context, volume, requester string) (string, error) {
	if m.status != nil {
		return m.status(ctx, volume, requester)
	}
	return "", nil
}

func (m *mockAgent) SendResumeBitmap(ctx context.Context, sessionID string, bitmap []byte) error {
	if m.sendBitmap != nil {
		return m.sendBitmap(ctx, sessionID, bitmap)
	}
	return nil
}

func (m *mockAgent) SendFinalManifest(ctx context.Context, sessionID string, manifest []byte) error {
	if m.sendManifest != nil {
		return m.sendManifest(ctx, sessionID, manifest)
	}
	return nil
}

func (m *mockAgent) Finalize(ctx context.Context, sessionID string) error {
	if m.finalizeSess != nil {
		return m.finalizeSess(ctx, sessionID)
	}
	return nil
}

func (m *mockAgent) Ack(ctx context.Context, ack *proto.Ack) (*proto.Ack, error) {
	if m.ack != nil {
		return m.ack(ctx, ack)
	}
	return ack, nil
}

func (m *mockAgent) Probe(ctx context.Context, volume string) error {
	if m.probe != nil {
		return m.probe(ctx, volume)
	}
	return nil
}

func (m *mockAgent) Cancel(ctx context.Context, sessionID string) error {
	if m.cancel != nil {
		return m.cancel(ctx, sessionID)
	}
	return nil
}

func (m *mockAgent) Progress(ctx context.Context, sessionID string) (<-chan *proto.Progress, error) {
	if m.progress != nil {
		return m.progress(ctx, sessionID)
	}
	ch := make(chan *proto.Progress)
	close(ch)
	return ch, nil
}

func (m *mockAgent) BuildManifest(ctx context.Context, sessionID string) error {
	if m.buildManifest != nil {
		return m.buildManifest(ctx, sessionID)
	}
	return nil
}

func (m *mockAgent) Verify(ctx context.Context, sessionID string) error {
	if m.verify != nil {
		return m.verify(ctx, sessionID)
	}
	return nil
}

func newClientWithLogger(t *testing.T, cfg Config, agent lvmagent.Agent, creds credentials.TransportCredentials, logger *zap.Logger) (proto.ReplicationClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv, srvCleanup, err := New(cfg, agent, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	go func(t *testing.T) {
		err := srv.Serve(lis)
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("srv.Serve: %v", err)
		}
	}(t)
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cleanup := func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn.Close: %v", err)
		}
		srv.Stop()
		srvCleanup()
	}
	return proto.NewReplicationClient(conn), cleanup
}

func newClient(t *testing.T, cfg Config, agent lvmagent.Agent, creds credentials.TransportCredentials) (proto.ReplicationClient, func()) {
	return newClientWithLogger(t, cfg, agent, creds, zap.NewNop())
}

func newAuthorizedClient(t *testing.T, agent lvmagent.Agent) (proto.ReplicationClient, func()) {
	cfg, good, _, _ := generateTLS(t)
	return newClient(t, cfg, agent, credentials.NewTLS(good))
}

func ctxWithRole(_ string) context.Context {
	return context.Background()
}

// runStatusTest executes an RPC returning a StatusResponse and verifies ok/message fields.
func runStatusTest(t *testing.T, agent lvmagent.Agent, ok bool, msg string, call func(proto.ReplicationClient) (*proto.StatusResponse, error)) {
	t.Helper()
	client, cleanup := newAuthorizedClient(t, agent)
	defer cleanup()
	resp, err := call(client)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp.GetOk() != ok || resp.GetMessage() != msg {
		t.Fatalf("unexpected response %#v", resp)
	}
}

type testCase struct {
	name  string
	agent lvmagent.Agent
	ok    bool
	msg   string
}

// runAgentTest executes an RPC for each test case and validates its StatusResponse.
func runAgentTest(t *testing.T, cases []testCase, call func(proto.ReplicationClient) (*proto.StatusResponse, error)) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runStatusTest(t, tc.agent, tc.ok, tc.msg, call)
		})
	}
}

func TestAuthorizeInterceptor(t *testing.T) {
	cfg, good, other, nocert := generateTLS(t)

	client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(good))
	defer cleanup()
	if _, err := client.Ping(context.Background(), &proto.Empty{}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("role", "replicator"))
	badClient, badCleanup := newClient(t, cfg, nil, credentials.NewTLS(other))
	defer badCleanup()
	if _, err := badClient.Ping(ctx, &proto.Empty{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}

	noCertClient, noCertCleanup := newClient(t, cfg, nil, credentials.NewTLS(nocert))
	defer noCertCleanup()
	if _, err := noCertClient.Ping(context.Background(), &proto.Empty{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLockVolume(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{}, true, ""},
		{"lock held", &mockAgent{lock: func(_ context.Context, _, _ string) error { return errors.New("already locked") }}, false, "already locked"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.LockVolume(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

//revive:disable-next-line:cognitive-complexity
func TestGetVolumeMetadata(t *testing.T) {
	tests := []struct {
		name    string
		agent   lvmagent.Agent
		wantErr bool
	}{
		{"success", &mockAgent{getMeta: func(_ context.Context, v string) (lvmagent.VolumeMetadata, error) {
			// ctx is unused.
			return lvmagent.VolumeMetadata{VolumeName: v, SizeBytes: 1, ChunkSize: 2}, nil
		}}, false},
		{"agent error", &mockAgent{getMeta: func(_ context.Context, _ string) (lvmagent.VolumeMetadata, error) {
			// parameters are unused in this mock.
			return lvmagent.VolumeMetadata{}, errors.New("fail")
		}}, true},
		{"no agent", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, cleanup := newAuthorizedClient(t, tt.agent)
			defer cleanup()
			resp, err := client.GetVolumeMetadata(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol"})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tt.agent == nil && status.Code(err) != codes.FailedPrecondition {
					t.Fatalf("expected failed precondition, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("call failed: %v", err)
				}
				if resp.GetVolumeName() != "vol" || resp.GetSizeBytes() != 1 || resp.GetChunkSize() != 2 {
					t.Fatalf("unexpected response %#v", resp)
				}
			}
		})
	}
}

func TestSendVolumeMetadata(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{}, true, ""},
		{"agent error", &mockAgent{sendMeta: func(_ context.Context, _ lvmagent.VolumeMetadata) error { return errors.New("checksum mismatch") }}, false, "checksum mismatch"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.SendVolumeMetadata(ctxWithRole("replicator"), &proto.VolumeMetadata{VolumeName: "vol"})
	})
}

func TestStartTransferSession(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{}, true, ""},
		{"agent error", &mockAgent{startSess: func(_ context.Context, _, _ string) error { return errors.New("session failed") }}, false, "session failed"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.StartTransferSession(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

func TestFinalizeSync(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{finalize: func(_ context.Context, _, _ string) error { return nil }, unlock: func(_ context.Context, _, _ string) error { return nil }}, true, ""},                                        // parameters unused
		{"finalize error", &mockAgent{finalize: func(_ context.Context, _, _ string) error { return errors.New("sync fail") }}, false, "sync fail"},                                                                      // parameters unused
		{"unlock error", &mockAgent{finalize: func(_ context.Context, _, _ string) error { return nil }, unlock: func(_ context.Context, _, _ string) error { return errors.New("unlock fail") }}, false, "unlock fail"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.FinalizeSync(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

func TestGetStatus(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{status: func(_ context.Context, _, _ string) (string, error) { return "ok", nil }}, true, "ok"},                   // parameters unused
		{"agent error", &mockAgent{status: func(_ context.Context, _, _ string) (string, error) { return "", errors.New("bad") }}, false, "bad"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.GetStatus(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

func TestSendFinalManifest(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{sendManifest: func(_ context.Context, id string, m []byte) error { return nil }}, true, ""},
		{"agent error", &mockAgent{sendManifest: func(_ context.Context, _ string, _ []byte) error { return errors.New("fail") }}, false, "fail"},
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.SendFinalManifest(ctxWithRole("replicator"), &proto.ManifestMessage{SessionId: "s", Manifest: []byte("{}")})
	})
}

func TestFinalize(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{finalizeSess: func(_ context.Context, _ string) error { return nil }}, true, ""},
		{"agent error", &mockAgent{finalizeSess: func(_ context.Context, _ string) error { return errors.New("fail") }}, false, "fail"},
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.Finalize(ctxWithRole("replicator"), &proto.FinalizeRequest{SessionId: "s"})
	})
}

func TestProbe(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{probe: func(_ context.Context, _ string) error { return nil }}, true, ""},
		{"agent error", &mockAgent{probe: func(_ context.Context, _ string) error { return errors.New("fail") }}, false, "fail"},
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.Probe(ctxWithRole("replicator"), &proto.ProbeRequest{VolumeName: "vol"})
	})
}

func TestStartSync(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{startSess: func(_ context.Context, _, _ string) error { return nil }}, true, ""},
		{"agent error", &mockAgent{startSess: func(_ context.Context, _, _ string) error { return errors.New("fail") }}, false, "fail"},
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.StartSync(ctxWithRole("replicator"), &proto.StartSyncRequest{VolumeName: "vol", Requester: "req"})
	})
}

func TestCancel(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{cancel: func(_ context.Context, _ string) error { return nil }}, true, ""},
		{"agent error", &mockAgent{cancel: func(_ context.Context, _ string) error { return errors.New("fail") }}, false, "fail"},
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.Cancel(ctxWithRole("replicator"), &proto.CancelRequest{SessionId: "s"})
	})
}

func TestBuildManifest(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{buildManifest: func(_ context.Context, _ string) error { return nil }}, true, ""},
		{"agent error", &mockAgent{buildManifest: func(_ context.Context, _ string) error { return errors.New("fail") }}, false, "fail"},
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.BuildManifest(ctxWithRole("replicator"), &proto.BuildManifestRequest{SessionId: "s"})
	})
}

func TestVerify(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{verify: func(_ context.Context, _ string) error { return nil }}, true, ""},
		{"agent error", &mockAgent{verify: func(_ context.Context, _ string) error { return errors.New("fail") }}, false, "fail"},
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.Verify(ctxWithRole("replicator"), &proto.VerifyRequest{SessionId: "s"})
	})
}

func TestSendResumeBitmap(t *testing.T) {
	ctx := ctxWithRole("replicator")
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{sendBitmap: func(_ context.Context, id string, _ []byte) error {
			if id != "s" {
				t.Fatalf("unexpected id %s", id)
			}
			return nil
		}}
		client, cleanup := newAuthorizedClient(t, agent)
		defer cleanup()
		stream, err := client.SendResumeBitmap(ctx)
		if err != nil {
			t.Fatalf("SendResumeBitmap: %v", err)
		}
		if err := stream.Send(&proto.ResumeBitmap{SessionId: "s", Bitmap: []byte{1}}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if _, err := stream.CloseAndRecv(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	t.Run("agent error", func(t *testing.T) {
		agent := &mockAgent{sendBitmap: func(_ context.Context, _ string, _ []byte) error { return errors.New("fail") }}
		client, cleanup := newAuthorizedClient(t, agent)
		defer cleanup()
		stream, err := client.SendResumeBitmap(ctx)
		if err != nil {
			t.Fatalf("SendResumeBitmap: %v", err)
		}
		if err := stream.Send(&proto.ResumeBitmap{SessionId: "s", Bitmap: []byte{1}}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if _, err := stream.CloseAndRecv(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newAuthorizedClient(t, nil)
		defer cleanup()
		stream, err := client.SendResumeBitmap(ctx)
		if err != nil {
			t.Fatalf("SendResumeBitmap: %v", err)
		}
		// The server should close immediately when no agent is configured.
		// The send should therefore return a non-nil error without needing to
		// call CloseAndRecv.
		time.Sleep(50 * time.Millisecond)
		err = stream.Send(&proto.ResumeBitmap{SessionId: "s", Bitmap: []byte{1}})
		if err == nil {
			t.Fatalf("expected error")
		}
		if status.Code(err) == codes.OK {
			t.Fatalf("unexpected OK code")
		}
	})
}

func TestAckStream(t *testing.T) {
	ctx := ctxWithRole("replicator")
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{ack: func(_ context.Context, ack *proto.Ack) (*proto.Ack, error) { return ack, nil }}
		client, cleanup := newAuthorizedClient(t, agent)
		defer cleanup()
		stream, err := client.AckStream(ctx)
		if err != nil {
			t.Fatalf("AckStream: %v", err)
		}
		if err := stream.Send(&proto.Ack{SessionId: "s", Ok: true}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if _, err := stream.Recv(); err != nil {
			t.Fatalf("recv: %v", err)
		}
	})

	t.Run("agent error", func(t *testing.T) {
		agent := &mockAgent{ack: func(_ context.Context, _ *proto.Ack) (*proto.Ack, error) { return nil, errors.New("fail") }}
		client, cleanup := newAuthorizedClient(t, agent)
		defer cleanup()
		stream, err := client.AckStream(ctx)
		if err != nil {
			t.Fatalf("AckStream: %v", err)
		}
		if err := stream.Send(&proto.Ack{SessionId: "s"}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if _, err := stream.Recv(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newAuthorizedClient(t, nil)
		defer cleanup()
		stream, err := client.AckStream(ctx)
		if err != nil {
			t.Fatalf("AckStream: %v", err)
		}
		if err := stream.Send(&proto.Ack{SessionId: "s"}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if _, err := stream.Recv(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestProgressStream(t *testing.T) {
	ctx := ctxWithRole("replicator")
	t.Run("success", func(t *testing.T) {
		ch := make(chan *proto.Progress, 1)
		ch <- &proto.Progress{SessionId: "s", Completed: 1, Total: 2}
		close(ch)
		agent := &mockAgent{progress: func(_ context.Context, id string) (<-chan *proto.Progress, error) {
			if id != "s" {
				t.Fatalf("unexpected id %s", id)
			}
			return ch, nil
		}}
		cfg, good, _, _ := generateTLS(t)
		core, obs := observer.New(zap.DebugLevel)
		logger := zap.New(core)
		client, cleanup := newClientWithLogger(t, cfg, agent, credentials.NewTLS(good), logger)
		defer cleanup()
		stream, err := client.ProgressStream(ctx, &proto.ProgressRequest{SessionId: "s"})
		if err != nil {
			t.Fatalf("ProgressStream: %v", err)
		}
		if _, err := stream.Recv(); err != nil {
			t.Fatalf("recv: %v", err)
		}
		logs := obs.All()
		if len(logs) != 1 {
			t.Fatalf("expected 1 log entry, got %d", len(logs))
		}
		fields := logs[0].ContextMap()
		if fields["session_id"] != "s" ||
			fields["completed_bytes"] != uint64(1) ||
			fields["total_bytes"] != uint64(2) {
			t.Fatalf("unexpected log fields: %v", fields)
		}
	})

	t.Run("agent error", func(t *testing.T) {
		agent := &mockAgent{progress: func(_ context.Context, _ string) (<-chan *proto.Progress, error) {
			return nil, errors.New("fail")
		}}
		client, cleanup := newAuthorizedClient(t, agent)
		defer cleanup()
		stream, err := client.ProgressStream(ctx, &proto.ProgressRequest{SessionId: "s"})
		if err != nil {
			t.Fatalf("ProgressStream: %v", err)
		}
		if _, err := stream.Recv(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newAuthorizedClient(t, nil)
		defer cleanup()
		stream, err := client.ProgressStream(ctx, &proto.ProgressRequest{SessionId: "s"})
		if err != nil {
			t.Fatalf("ProgressStream: %v", err)
		}
		if _, err := stream.Recv(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestBuildManifestLogs(t *testing.T) {
	ctx := ctxWithRole("replicator")
	called := false
	agent := &mockAgent{buildManifest: func(_ context.Context, id string) error {
		if id != "s" {
			t.Fatalf("unexpected id %s", id)
		}
		called = true
		return nil
	}}
	cfg, good, _, _ := generateTLS(t)
	core, obs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	client, cleanup := newClientWithLogger(t, cfg, agent, credentials.NewTLS(good), logger)
	defer cleanup()
	if _, err := client.BuildManifest(ctx, &proto.BuildManifestRequest{SessionId: "s"}); err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if !called {
		t.Fatalf("agent not called")
	}
	logs := obs.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Message != "build_manifest" {
		t.Fatalf("unexpected message %q", logs[0].Message)
	}
	fields := logs[0].ContextMap()
	if fields["session_id"] != "s" {
		t.Fatalf("unexpected log fields: %v", fields)
	}
}

func TestVerifyLogs(t *testing.T) {
	ctx := ctxWithRole("replicator")
	called := false
	agent := &mockAgent{verify: func(_ context.Context, id string) error {
		if id != "s" {
			t.Fatalf("unexpected id %s", id)
		}
		called = true
		return nil
	}}
	cfg, good, _, _ := generateTLS(t)
	core, obs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	client, cleanup := newClientWithLogger(t, cfg, agent, credentials.NewTLS(good), logger)
	defer cleanup()
	if _, err := client.Verify(ctx, &proto.VerifyRequest{SessionId: "s"}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !called {
		t.Fatalf("agent not called")
	}
	logs := obs.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Message != "verify_request" {
		t.Fatalf("unexpected message %q", logs[0].Message)
	}
	fields := logs[0].ContextMap()
	if fields["session_id"] != "s" {
		t.Fatalf("unexpected log fields: %v", fields)
	}
}

func generateTLS(t *testing.T) (Config, *tls.Config, *tls.Config, *tls.Config) {
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	serverCert := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"server"},
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})

	goodCert := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "client", OrganizationalUnit: []string{"replicator"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	goodKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	goodDER, err := x509.CreateCertificate(rand.Reader, goodCert, ca, &goodKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	goodCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: goodDER})
	goodKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(goodKey)})

	otherCert := &x509.Certificate{
		SerialNumber: big.NewInt(4),
		Subject:      pkix.Name{CommonName: "client", OrganizationalUnit: []string{"other"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	otherDER, err := x509.CreateCertificate(rand.Reader, otherCert, ca, &otherKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	otherCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: otherDER})
	otherKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(otherKey)})

	dir := t.TempDir()
	srvCertFile := filepath.Join(dir, "server.pem")
	srvKeyFile := filepath.Join(dir, "server.key")
	caFile := filepath.Join(dir, "ca.pem")
	err = os.WriteFile(srvCertFile, serverCertPEM, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(srvKeyFile, serverKeyPEM, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(caFile, caPEM, 0600)
	if err != nil {
		t.Fatal(err)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	goodTLSCert, err := tls.X509KeyPair(goodCertPEM, goodKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	otherTLSCert, err := tls.X509KeyPair(otherCertPEM, otherKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	good := &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{goodTLSCert}, MinVersion: tls.VersionTLS13, ServerName: "server"}
	otherCfg := &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{otherTLSCert}, MinVersion: tls.VersionTLS13, ServerName: "server"}
	nocert := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13, ServerName: "server"}

	cfg := Config{TLSCert: srvCertFile, TLSKey: srvKeyFile, CACert: caFile}
	return cfg, good, otherCfg, nocert
}

func TestMTLSValidation(t *testing.T) {
	cfg, good, _, bad := generateTLS(t)

	t.Run("valid client cert", func(t *testing.T) {
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(good))
		defer cleanup()
		if _, err := client.Ping(context.Background(), &proto.Empty{}); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})

	t.Run("missing client cert", func(t *testing.T) {
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(bad))
		defer cleanup()
		if _, err := client.Ping(context.Background(), &proto.Empty{}); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestSessionFlow(t *testing.T) {
	agent := &mockAgent{sendBitmap: func(_ context.Context, _ string, _ []byte) error { return nil }, sendManifest: func(_ context.Context, _ string, _ []byte) error { return nil }, finalizeSess: func(_ context.Context, _ string) error { return nil }, ack: func(_ context.Context, a *proto.Ack) (*proto.Ack, error) { return a, nil }}
	client, cleanup := newAuthorizedClient(t, agent)
	defer cleanup()
	ctx := ctxWithRole("replicator")
	hsResp, err := client.Handshake(ctx, &proto.HandshakeRequest{SectorSize: 512, Alignment: 512, MaxConcurrency: 1})
	if err != nil || !hsResp.GetOk() {
		t.Fatalf("Handshake: %v", err)
	}
	sess, err := client.CreateSession(ctx, &proto.SessionRequest{VolumeName: "vol", DeviceUuid: "dev", ClientCert: dummyCert(t)})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if len(sess.GetPsk()) == 0 {
		t.Fatalf("expected psk in session response")
	}
	bmp, err := client.SendResumeBitmap(ctx)
	if err != nil {
		t.Fatalf("SendResumeBitmap: %v", err)
	}
	if err := bmp.Send(&proto.ResumeBitmap{SessionId: sess.GetSessionId(), Bitmap: []byte{1}}); err != nil {
		t.Fatalf("bitmap send: %v", err)
	}
	if _, err := bmp.CloseAndRecv(); err != nil {
		t.Fatalf("bitmap close: %v", err)
	}
	if _, err := client.SendFinalManifest(ctx, &proto.ManifestMessage{SessionId: sess.GetSessionId(), Manifest: []byte("{}")}); err != nil {
		t.Fatalf("SendFinalManifest: %v", err)
	}
	if _, err := client.Finalize(ctx, &proto.FinalizeRequest{SessionId: sess.GetSessionId()}); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	ack, err := client.AckStream(ctx)
	if err != nil {
		t.Fatalf("AckStream: %v", err)
	}
	if err := ack.Send(&proto.Ack{SessionId: sess.GetSessionId(), Ok: true, Message: "ping"}); err != nil {
		t.Fatalf("ack send: %v", err)
	}
	if _, err := ack.Recv(); err != nil {
		t.Fatalf("ack recv: %v", err)
	}
}

func TestHandshakeFailure(t *testing.T) {
	client, cleanup := newAuthorizedClient(t, nil)
	defer cleanup()
	ctx := ctxWithRole("replicator")
	if _, err := client.Handshake(ctx, &proto.HandshakeRequest{}); err == nil {
		t.Fatalf("expected handshake error")
	}
}

func TestNewRPCPermissions(t *testing.T) {
	t.Run("probe", func(t *testing.T) {
		cfg, good, other, _ := generateTLS(t)
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(good))
		defer cleanup()
		if _, err := client.Probe(context.Background(), &proto.ProbeRequest{VolumeName: "vol"}); err != nil {
			t.Fatalf("Probe: %v", err)
		}
		unauth, unauthCleanup := newClient(t, cfg, nil, credentials.NewTLS(other))
		defer unauthCleanup()
		if _, err := unauth.Probe(context.Background(), &proto.ProbeRequest{VolumeName: "vol"}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected permission denied, got %v", err)
		}
	})

	t.Run("startsync", func(t *testing.T) {
		agent := &mockAgent{startSess: func(_ context.Context, v, r string) error {
			if v != "vol" || r != "req" {
				t.Fatalf("unexpected params %s %s", v, r)
			}
			return nil
		}}
		cfg, good, other, _ := generateTLS(t)
		client, cleanup := newClient(t, cfg, agent, credentials.NewTLS(good))
		defer cleanup()
		if _, err := client.StartSync(context.Background(), &proto.StartSyncRequest{VolumeName: "vol", Requester: "req"}); err != nil {
			t.Fatalf("StartSync: %v", err)
		}
		unauth, unauthCleanup := newClient(t, cfg, agent, credentials.NewTLS(other))
		defer unauthCleanup()
		if _, err := unauth.StartSync(context.Background(), &proto.StartSyncRequest{VolumeName: "vol", Requester: "req"}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected permission denied, got %v", err)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		cfg, good, other, _ := generateTLS(t)
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(good))
		defer cleanup()
		if _, err := client.Cancel(context.Background(), &proto.CancelRequest{SessionId: "s"}); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		unauth, unauthCleanup := newClient(t, cfg, nil, credentials.NewTLS(other))
		defer unauthCleanup()
		if _, err := unauth.Cancel(context.Background(), &proto.CancelRequest{SessionId: "s"}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected permission denied, got %v", err)
		}
	})

	t.Run("progress", func(t *testing.T) {
		agent := &mockAgent{progress: func(_ context.Context, _ string) (<-chan *proto.Progress, error) {
			ch := make(chan *proto.Progress, 1)
			ch <- &proto.Progress{SessionId: "s"}
			close(ch)
			return ch, nil
		}}
		cfg, good, other, _ := generateTLS(t)
		client, cleanup := newClient(t, cfg, agent, credentials.NewTLS(good))
		defer cleanup()
		stream, err := client.ProgressStream(context.Background(), &proto.ProgressRequest{SessionId: "s"})
		if err != nil {
			t.Fatalf("ProgressStream: %v", err)
		}
		if _, err := stream.Recv(); err != nil {
			t.Fatalf("Progress recv: %v", err)
		}
		unauth, unauthCleanup := newClient(t, cfg, agent, credentials.NewTLS(other))
		defer unauthCleanup()
		if us, err := unauth.ProgressStream(context.Background(), &proto.ProgressRequest{SessionId: "s"}); err == nil {
			if _, err := us.Recv(); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("expected permission denied, got %v", err)
			}
		}
	})

	t.Run("buildmanifest", func(t *testing.T) {
		cfg, good, other, _ := generateTLS(t)
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(good))
		defer cleanup()
		if _, err := client.BuildManifest(context.Background(), &proto.BuildManifestRequest{SessionId: "s"}); err != nil {
			t.Fatalf("BuildManifest: %v", err)
		}
		unauth, unauthCleanup := newClient(t, cfg, nil, credentials.NewTLS(other))
		defer unauthCleanup()
		if _, err := unauth.BuildManifest(context.Background(), &proto.BuildManifestRequest{SessionId: "s"}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected permission denied, got %v", err)
		}
	})

	t.Run("verify", func(t *testing.T) {
		cfg, good, other, _ := generateTLS(t)
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(good))
		defer cleanup()
		if _, err := client.Verify(context.Background(), &proto.VerifyRequest{SessionId: "s"}); err != nil {
			t.Fatalf("Verify: %v", err)
		}
		unauth, unauthCleanup := newClient(t, cfg, nil, credentials.NewTLS(other))
		defer unauthCleanup()
		if _, err := unauth.Verify(context.Background(), &proto.VerifyRequest{SessionId: "s"}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected permission denied, got %v", err)
		}
	})
}

func TestNewRPCsMTLS(t *testing.T) {
	cfg, good, _, bad := generateTLS(t)
	agent := &mockAgent{progress: func(_ context.Context, _ string) (<-chan *proto.Progress, error) {
		ch := make(chan *proto.Progress, 1)
		ch <- &proto.Progress{SessionId: "s"}
		close(ch)
		return ch, nil
	}}
	client, cleanup := newClient(t, cfg, agent, credentials.NewTLS(good))
	defer cleanup()
	if _, err := client.Probe(context.Background(), &proto.ProbeRequest{VolumeName: "vol"}); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if _, err := client.StartSync(context.Background(), &proto.StartSyncRequest{VolumeName: "vol", Requester: "req"}); err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if _, err := client.Cancel(context.Background(), &proto.CancelRequest{SessionId: "s"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	ps, err := client.ProgressStream(context.Background(), &proto.ProgressRequest{SessionId: "s"})
	if err != nil {
		t.Fatalf("ProgressStream: %v", err)
	}
	if _, err := ps.Recv(); err != nil {
		t.Fatalf("Progress recv: %v", err)
	}
	if _, err := client.BuildManifest(context.Background(), &proto.BuildManifestRequest{SessionId: "s"}); err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if _, err := client.Verify(context.Background(), &proto.VerifyRequest{SessionId: "s"}); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	badClient, badCleanup := newClient(t, cfg, nil, credentials.NewTLS(bad))
	defer badCleanup()
	if _, err := badClient.Probe(context.Background(), &proto.ProbeRequest{VolumeName: "vol"}); err == nil {
		t.Fatalf("expected handshake failure")
	}
}

func TestPlaintextRejected(t *testing.T) {
	cfg, _, _, _ := generateTLS(t)
	lis := bufconn.Listen(bufSize)
	srv, srvCleanup, err := New(cfg, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	go srv.Serve(lis)
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	client := proto.NewReplicationClient(conn)
	if _, err := client.Ping(context.Background(), &proto.Empty{}); err == nil {
		t.Fatalf("expected handshake failure")
	}
	conn.Close()
	srv.Stop()
	srvCleanup()
}

func dummyCert(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return der
}

func TestNewTLSFailures(t *testing.T) {
	cfg, _, _, _ := generateTLS(t)

	t.Run("missing cert", func(t *testing.T) {
		bad := cfg
		bad.TLSCert = ""
		if _, _, err := New(bad, nil, zap.NewNop()); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing key", func(t *testing.T) {
		bad := cfg
		bad.TLSKey = ""
		if _, _, err := New(bad, nil, zap.NewNop()); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing CA", func(t *testing.T) {
		bad := cfg
		bad.CACert = ""
		if _, _, err := New(bad, nil, zap.NewNop()); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalid CA", func(t *testing.T) {
		bad := cfg
		invalid := filepath.Join(t.TempDir(), "bad.pem")
		if err := os.WriteFile(invalid, []byte("invalid"), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		bad.CACert = invalid
		if _, _, err := New(bad, nil, zap.NewNop()); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestRequestTimeout(t *testing.T) {
	cfg, good, _, _ := generateTLS(t)
	cfg.RequestTimeout = 50 * time.Millisecond
	agent := &mockAgent{probe: func(ctx context.Context, _ string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return nil
		}
	}}
	client, cleanup := newClient(t, cfg, agent, credentials.NewTLS(good))
	defer cleanup()
	_, err := client.Probe(context.Background(), &proto.ProbeRequest{VolumeName: "vol"})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
