package transport

import (
	"crypto/tls"
	"testing"
)

func TestTLSVersionString(t *testing.T) {
	cases := map[uint16]string{
		tls.VersionTLS10: "1.0",
		tls.VersionTLS11: "1.1",
		tls.VersionTLS12: "1.2",
		tls.VersionTLS13: "1.3",
		0:                "",
	}
	for v, want := range cases {
		if got := TLSVersionString(v); got != want {
			t.Fatalf("TLSVersionString(%d)=%q want %q", v, got, want)
		}
	}
}
