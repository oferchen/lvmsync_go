package transport_test

import (
	"testing"

	"github.com/oferchen/lvmsync_go/common"
	_ "github.com/oferchen/lvmsync_go/transport/h2"
	_ "github.com/oferchen/lvmsync_go/transport/quic"
	_ "github.com/oferchen/lvmsync_go/transport/ssh"
	_ "github.com/oferchen/lvmsync_go/transport/tcp_tls"
	"github.com/oferchen/lvmsync_go/transport/testutil"
)

func TestTransportNegotiationMatrix(t *testing.T) {
	transports := []string{"ssh", "tcp+tls", "h2", "quic"}
	for _, name := range transports {
		t.Run(name, func(t *testing.T) {
			tr := testutil.NewTransport(t, name)
			base := common.Handshake{
				DedupMode:     "fixed",
				CDCMin:        64,
				CDCAvg:        128,
				CDCMax:        256,
				Compress:      "zstd",
				CompressLevel: 1,
				ODirect:       true,
				ResumeToken:   "tok",
				MaxInFlight:   8,
				Endianness:    common.NativeEndianness(),
				ALPN:          "lvmsync",
				TLSVersion:    "1.3",
				CRC32C:        true,
			}
			if name == "h2" {
				base.ALPN = "h2"
			}
			modes := []string{"fixed", "cdc", "hybrid"}
			for _, m := range modes {
				serverHS := base
				clientHS := base
				serverHS.DedupMode = m
				clientHS.DedupMode = m
				srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS)
				if srv.Err != nil || cli.Err != nil {
					t.Fatalf("expected success: server %v client %v", srv.Err, cli.Err)
				}
				if cli.Peer.DedupMode != m || cli.Peer.Compress != "zstd" || cli.Peer.ODirect != true ||
					cli.Peer.CDCMin != base.CDCMin || cli.Peer.CDCAvg != base.CDCAvg || cli.Peer.CDCMax != base.CDCMax ||
					cli.Peer.ALPN != base.ALPN || cli.Peer.TLSVersion != base.TLSVersion {
					t.Fatalf("unexpected client peer handshake: %+v", cli.Peer)
				}
				if srv.Peer.DedupMode != m || srv.Peer.Compress != "zstd" || srv.Peer.ODirect != true ||
					srv.Peer.CDCMin != base.CDCMin || srv.Peer.CDCAvg != base.CDCAvg || srv.Peer.CDCMax != base.CDCMax ||
					srv.Peer.ALPN != base.ALPN || srv.Peer.TLSVersion != base.TLSVersion {
					t.Fatalf("unexpected server peer handshake: %+v", srv.Peer)
				}
			}
			// mismatch cases
			// dedup mode
			serverHS := base
			clientHS := base
			clientHS.DedupMode = "cdc"
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected dedup mismatch error")
			}
			// cdc min
			serverHS = base
			serverHS.DedupMode = "cdc"
			clientHS = serverHS
			clientHS.CDCMin = 128
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected cdc min mismatch error")
			}
			// cdc avg
			serverHS = base
			serverHS.DedupMode = "cdc"
			clientHS = serverHS
			clientHS.CDCAvg = 256
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected cdc avg mismatch error")
			}
			// cdc max
			serverHS = base
			serverHS.DedupMode = "cdc"
			clientHS = serverHS
			clientHS.CDCMax = 512
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected cdc max mismatch error")
			}
			// compression algorithm
			serverHS = base
			clientHS = base
			clientHS.Compress = "lz4"
			clientHS.CompressLevel = 0
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected compression mismatch error")
			}
			// compression level
			serverHS = base
			clientHS = base
			clientHS.CompressLevel = 2
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected compression level mismatch error")
			}
			// O_DIRECT
			serverHS = base
			clientHS = base
			clientHS.ODirect = false
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected o_direct mismatch error")
			}
			// resume token
			serverHS = base
			clientHS = base
			clientHS.ResumeToken = "other"
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected resume token mismatch error")
			}
			// max in-flight
			serverHS = base
			clientHS = base
			clientHS.MaxInFlight = 4
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected max in-flight mismatch error")
			}
			// ALPN
			serverHS = base
			clientHS = base
			clientHS.ALPN = "other"
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected alpn mismatch error")
			}
			// TLS version
			serverHS = base
			clientHS = base
			clientHS.TLSVersion = "1.2"
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected tls version mismatch error")
			}
			// digest algorithm
			serverHS = base
			serverHS.Digest = "blake3"
			clientHS = base
			clientHS.Digest = "sha256"
			if srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS); srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected digest mismatch error")
			}
		})
	}
}
