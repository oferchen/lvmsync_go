package auth

import (
	"context"
	"crypto/x509"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Role describes an allowed certificate issuer and subject pair.
type Role struct {
	Issuer  string
	Subject string
}

// NewAuthInterceptor returns a grpc.UnaryServerInterceptor that enforces
// client certificate issuer and subject to match the provided role.
func NewAuthInterceptor(role Role) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		p, ok := peer.FromContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing peer info")
		}

		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing TLS info")
		}

		state := tlsInfo.State
		if len(state.PeerCertificates) == 0 {
			return nil, status.Error(codes.Unauthenticated, "no client certificate")
		}
		cert := state.PeerCertificates[0]

		if !matchRole(cert, role) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized role")
		}

		return handler(ctx, req)
	}
}

func matchRole(cert *x509.Certificate, role Role) bool {
	if cert == nil {
		return false
	}
	issuer := cert.Issuer.CommonName
	subject := cert.Subject.CommonName
	return issuer == role.Issuer && subject == role.Subject
}
