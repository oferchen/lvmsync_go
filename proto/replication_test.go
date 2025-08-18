package proto

import (
	"reflect"
	"testing"

	"google.golang.org/grpc"
	gproto "google.golang.org/protobuf/proto"
)

type dummyReplicationServer struct {
	UnimplementedReplicationServer
}

func TestRegisterReplicationServer(t *testing.T) {
	srv := grpc.NewServer()
	RegisterReplicationServer(srv, &dummyReplicationServer{})
	info := srv.GetServiceInfo()
	if _, ok := info["replication.Replication"]; !ok {
		t.Fatalf("Replication service not registered")
	}
}

func TestReplicationMessagesRoundTrip(t *testing.T) {
	cases := []gproto.Message{
		&VolumeMetadata{VolumeName: "vol", SizeBytes: 1, ChunkSize: 2},
		&LockRequest{VolumeName: "vol", Requester: "alice"},
		&StatusResponse{Ok: true, Message: "ok"},
		&Empty{},
		&HandshakeRequest{SectorSize: 512, Alignment: 4096, MaxConcurrency: 8, DedupSupported: true, CompressionSupported: true},
		&HandshakeResponse{Ok: true, Message: "hi"},
		&SessionRequest{VolumeName: "vol", DeviceUuid: "uuid", ClientCert: []byte("cert")},
		&SessionResponse{SessionId: "id", Psk: []byte("psk"), ServerCert: []byte("servercert")},
		&ResumeBitmap{SessionId: "id", Bitmap: []byte{0, 1, 2}},
		&ManifestMessage{SessionId: "id", Manifest: []byte("manifest")},
		&FinalizeRequest{SessionId: "id"},
		&Ack{SessionId: "id", Ok: true, Message: "done"},
		&ProbeRequest{VolumeName: "vol"},
		&StartSyncRequest{VolumeName: "vol", Requester: "req"},
		&CancelRequest{SessionId: "id"},
		&ProgressRequest{SessionId: "id"},
		&Progress{SessionId: "id", Completed: 10, Total: 20},
		&BuildManifestRequest{SessionId: "id"},
		&VerifyRequest{SessionId: "id"},
	}

	for _, msg := range cases {
		data, err := gproto.Marshal(msg)
		if err != nil {
			t.Fatalf("marshal %T: %v", msg, err)
		}
		newMsg := reflect.New(reflect.TypeOf(msg).Elem()).Interface().(gproto.Message)
		if err := gproto.Unmarshal(data, newMsg); err != nil {
			t.Fatalf("unmarshal %T: %v", msg, err)
		}
		if !gproto.Equal(msg, newMsg) {
			t.Fatalf("round trip mismatch for %T", msg)
		}

		switch m := newMsg.(type) {
		case *VolumeMetadata:
			if m.GetVolumeName() != "vol" || m.GetSizeBytes() != 1 || m.GetChunkSize() != 2 {
				t.Fatalf("unexpected VolumeMetadata: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *LockRequest:
			if m.GetVolumeName() != "vol" || m.GetRequester() != "alice" {
				t.Fatalf("unexpected LockRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *StatusResponse:
			if !m.GetOk() || m.GetMessage() != "ok" {
				t.Fatalf("unexpected StatusResponse: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *Empty:
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *HandshakeRequest:
			if m.GetSectorSize() != 512 || m.GetAlignment() != 4096 || m.GetMaxConcurrency() != 8 || !m.GetDedupSupported() || !m.GetCompressionSupported() {
				t.Fatalf("unexpected HandshakeRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *HandshakeResponse:
			if !m.GetOk() || m.GetMessage() != "hi" {
				t.Fatalf("unexpected HandshakeResponse: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *SessionRequest:
			if m.GetVolumeName() != "vol" || m.GetDeviceUuid() != "uuid" || string(m.GetClientCert()) != "cert" {
				t.Fatalf("unexpected SessionRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *SessionResponse:
			if m.GetSessionId() != "id" || string(m.GetPsk()) != "psk" || string(m.GetServerCert()) != "servercert" {
				t.Fatalf("unexpected SessionResponse: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *ResumeBitmap:
			if m.GetSessionId() != "id" || len(m.GetBitmap()) != 3 {
				t.Fatalf("unexpected ResumeBitmap: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *ManifestMessage:
			if m.GetSessionId() != "id" || string(m.GetManifest()) != "manifest" {
				t.Fatalf("unexpected ManifestMessage: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *FinalizeRequest:
			if m.GetSessionId() != "id" {
				t.Fatalf("unexpected FinalizeRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *Ack:
			if m.GetSessionId() != "id" || !m.GetOk() || m.GetMessage() != "done" {
				t.Fatalf("unexpected Ack: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *ProbeRequest:
			if m.GetVolumeName() != "vol" {
				t.Fatalf("unexpected ProbeRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *StartSyncRequest:
			if m.GetVolumeName() != "vol" || m.GetRequester() != "req" {
				t.Fatalf("unexpected StartSyncRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *CancelRequest:
			if m.GetSessionId() != "id" {
				t.Fatalf("unexpected CancelRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *ProgressRequest:
			if m.GetSessionId() != "id" {
				t.Fatalf("unexpected ProgressRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *Progress:
			if m.GetSessionId() != "id" || m.GetCompleted() != 10 || m.GetTotal() != 20 {
				t.Fatalf("unexpected Progress: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *BuildManifestRequest:
			if m.GetSessionId() != "id" {
				t.Fatalf("unexpected BuildManifestRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
			m.Descriptor()
		case *VerifyRequest:
			if m.GetSessionId() != "id" {
				t.Fatalf("unexpected VerifyRequest: %+v", m)
			}
			_ = m.String()
			m.Reset()
			m.ProtoReflect()
		}
	}
}
