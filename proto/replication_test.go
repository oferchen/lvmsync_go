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
	}
}
