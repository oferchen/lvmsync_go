package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestAuthInterceptor(t *testing.T) {
	role := Role{Issuer: "issuer", Subject: "subject"}
	interceptor := NewAuthInterceptor(role)

	mkCtx := func(cert *x509.Certificate) context.Context {
		state := tls.ConnectionState{}
		if cert != nil {
			state.PeerCertificates = []*x509.Certificate{cert}
		}
		p := &peer.Peer{AuthInfo: credentials.TLSInfo{State: state}}
		return peer.NewContext(context.Background(), p)
	}

	handler := func(_ context.Context, _ interface{}) (interface{}, error) {
		// ctx and req are unused in this test handler.
		return "ok", nil
	}

	t.Run("allowed", func(t *testing.T) {
		cert := &x509.Certificate{
			Issuer:  pkixName("issuer"),
			Subject: pkixName("subject"),
		}
		resp, err := interceptor(mkCtx(cert), nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if resp.(string) != "ok" {
			t.Fatalf("unexpected response %v", resp)
		}
	})

	t.Run("mismatched", func(t *testing.T) {
		cert := &x509.Certificate{
			Issuer:  pkixName("other"),
			Subject: pkixName("subject"),
		}
		_, err := interceptor(mkCtx(cert), nil, &grpc.UnaryServerInfo{}, handler)
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected permission denied, got %v", err)
		}
	})

	t.Run("missing cert", func(t *testing.T) {
		_, err := interceptor(mkCtx(nil), nil, &grpc.UnaryServerInfo{}, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected unauthenticated, got %v", err)
		}
	})

	t.Run("missing peer", func(t *testing.T) {
		_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected unauthenticated, got %v", err)
		}
	})

	t.Run("missing TLS info", func(t *testing.T) {
		p := &peer.Peer{}
		ctx := peer.NewContext(context.Background(), p)
		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected unauthenticated, got %v", err)
		}
	})
}

func pkixName(cn string) pkix.Name {
	return pkix.Name{CommonName: cn}
}
