package proto

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	gproto "google.golang.org/protobuf/proto"
)

type fullServer struct{ UnimplementedReplicationServer }

func (fullServer) LockVolume(context.Context, *LockRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) GetVolumeMetadata(context.Context, *LockRequest) (*VolumeMetadata, error) {
	return &VolumeMetadata{VolumeName: "vol", SizeBytes: 1, ChunkSize: 2}, nil
}
func (fullServer) SendVolumeMetadata(context.Context, *VolumeMetadata) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) StartTransferSession(context.Context, *LockRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) FinalizeSync(context.Context, *LockRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) GetStatus(context.Context, *LockRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) Ping(context.Context, *Empty) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) Handshake(context.Context, *HandshakeRequest) (*HandshakeResponse, error) {
	return &HandshakeResponse{Ok: true, Message: "hi"}, nil
}
func (fullServer) CreateSession(context.Context, *SessionRequest) (*SessionResponse, error) {
	return &SessionResponse{SessionId: "id"}, nil
}
func (fullServer) SendResumeBitmap(stream grpc.ClientStreamingServer[ResumeBitmap, StatusResponse]) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&StatusResponse{Ok: true})
		}
		if err != nil {
			return err
		}
	}
}
func (fullServer) SendFinalManifest(context.Context, *ManifestMessage) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) Finalize(context.Context, *FinalizeRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) AckStream(stream grpc.BidiStreamingServer[Ack, Ack]) error {
	for {
		m, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(m); err != nil {
			return err
		}
	}
}
func (fullServer) Probe(ctx context.Context, req *ProbeRequest) (*StatusResponse, error) {
	if req.GetVolumeName() != "vol" {
		return nil, fmt.Errorf("unexpected volume %q", req.GetVolumeName())
	}
	return &StatusResponse{Ok: true, Message: "pong"}, nil
}
func (fullServer) StartSync(context.Context, *StartSyncRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) Cancel(context.Context, *CancelRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) ProgressStream(req *ProgressRequest, stream grpc.ServerStreamingServer[Progress]) error {
	return stream.Send(&Progress{SessionId: req.GetSessionId(), Completed: 1, Total: 2})
}
func (fullServer) BuildManifest(context.Context, *BuildManifestRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}
func (fullServer) Verify(context.Context, *VerifyRequest) (*StatusResponse, error) {
	return &StatusResponse{Ok: true}, nil
}

func TestMarshalUnmarshalHandshake(t *testing.T) {
	orig := &HandshakeRequest{SectorSize: 512, Alignment: 4096, MaxConcurrency: 2, DedupSupported: true, CompressionSupported: true}
	data, err := gproto.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded HandshakeRequest
	if err := gproto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.GetSectorSize() != orig.SectorSize || decoded.GetAlignment() != orig.Alignment || decoded.GetMaxConcurrency() != orig.MaxConcurrency || decoded.GetDedupSupported() != orig.DedupSupported || decoded.GetCompressionSupported() != orig.CompressionSupported {
		t.Fatalf("unexpected decoded %+v", decoded)
	}
}

func TestMarshalUnmarshalAck(t *testing.T) {
	orig := &Ack{SessionId: "id", Ok: true, Message: "done"}
	data, err := gproto.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Ack
	if err := gproto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.GetSessionId() != orig.SessionId || decoded.GetOk() != orig.Ok || decoded.GetMessage() != orig.Message {
		t.Fatalf("unexpected decoded %+v", decoded)
	}
}

func TestProbeRoundTrip(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	RegisterReplicationServer(srv, fullServer{})
	go func() { _ = srv.Serve(lis) }()
	ctx := context.Background()
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := NewReplicationClient(conn)
	if _, err := client.LockVolume(ctx, &LockRequest{}); err != nil {
		t.Fatalf("LockVolume: %v", err)
	}
	if _, err := client.GetVolumeMetadata(ctx, &LockRequest{}); err != nil {
		t.Fatalf("GetVolumeMetadata: %v", err)
	}
	if _, err := client.SendVolumeMetadata(ctx, &VolumeMetadata{}); err != nil {
		t.Fatalf("SendVolumeMetadata: %v", err)
	}
	if _, err := client.StartTransferSession(ctx, &LockRequest{}); err != nil {
		t.Fatalf("StartTransferSession: %v", err)
	}
	if _, err := client.FinalizeSync(ctx, &LockRequest{}); err != nil {
		t.Fatalf("FinalizeSync: %v", err)
	}
	if _, err := client.GetStatus(ctx, &LockRequest{}); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if _, err := client.Ping(ctx, &Empty{}); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if _, err := client.Handshake(ctx, &HandshakeRequest{}); err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if _, err := client.CreateSession(ctx, &SessionRequest{}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	rs, err := client.SendResumeBitmap(ctx)
	if err != nil {
		t.Fatalf("SendResumeBitmap: %v", err)
	}
	if err := rs.Send(&ResumeBitmap{SessionId: "id"}); err != nil {
		t.Fatalf("SendResumeBitmap send: %v", err)
	}
	if _, err := rs.CloseAndRecv(); err != nil {
		t.Fatalf("SendResumeBitmap close: %v", err)
	}
	if _, err := client.SendFinalManifest(ctx, &ManifestMessage{}); err != nil {
		t.Fatalf("SendFinalManifest: %v", err)
	}
	if _, err := client.Finalize(ctx, &FinalizeRequest{}); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	as, err := client.AckStream(ctx)
	if err != nil {
		t.Fatalf("AckStream: %v", err)
	}
	if err := as.Send(&Ack{SessionId: "id"}); err != nil {
		t.Fatalf("AckStream send: %v", err)
	}
	if _, err := as.Recv(); err != nil {
		t.Fatalf("AckStream recv: %v", err)
	}
	if err := as.CloseSend(); err != nil {
		t.Fatalf("AckStream close: %v", err)
	}
	resp, err := client.Probe(ctx, &ProbeRequest{VolumeName: "vol"})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !resp.GetOk() || resp.GetMessage() != "pong" {
		t.Fatalf("unexpected Probe response %+v", resp)
	}
	if _, err := client.StartSync(ctx, &StartSyncRequest{}); err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if _, err := client.Cancel(ctx, &CancelRequest{}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	ps, err := client.ProgressStream(ctx, &ProgressRequest{SessionId: "id"})
	if err != nil {
		t.Fatalf("ProgressStream: %v", err)
	}
	if _, err := ps.Recv(); err != nil {
		t.Fatalf("ProgressStream recv: %v", err)
	}
	if _, err := client.BuildManifest(ctx, &BuildManifestRequest{}); err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if _, err := client.Verify(ctx, &VerifyRequest{}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}
