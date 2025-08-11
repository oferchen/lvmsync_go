package quic

import (
	"bufio"
	"net"
	"testing"
)

func TestNegotiate(t *testing.T) {
	server, client := net.Pipe()
	expected := Negotiation{Protocol: "proto", Algorithm: "alg", TestSpace: "ts"}
	errCh := make(chan error, 1)
	go func() {
		errCh <- Negotiate(server, expected)
		server.Close()
	}()
	if err := WriteNegotiation(client, expected); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	got, err := ReadNegotiation(bufio.NewReader(client))
	if err != nil {
		t.Fatalf("read negotiation: %v", err)
	}
	if got != expected {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("negotiate error: %v", err)
	}
}

func TestNegotiateMismatch(t *testing.T) {
	server, client := net.Pipe()
	expected := Negotiation{Protocol: "proto", Algorithm: "alg", TestSpace: "ts"}
	errCh := make(chan error, 1)
	go func() {
		errCh <- Negotiate(server, expected)
		server.Close()
	}()
	if err := WriteNegotiation(client, Negotiation{Protocol: "proto", Algorithm: "other", TestSpace: "ts"}); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	_ = client.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected error")
	}
}
