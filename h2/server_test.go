package h2

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/net/http2"
)

func TestUploadDownload(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()
	srv := NewServer(logger)
	ts := httptest.NewUnstartedServer(srv.Handler())
	http2.ConfigureServer(ts.Config, &http2.Server{})
	ts.TLS = ts.Config.TLSConfig
	ts.StartTLS()
	defer ts.Close()

	client := &http.Client{Transport: &http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

	data := []byte("HelloWorld")

	// upload first chunk
	req1, err := http.NewRequest("PUT", ts.URL+"/upload/s1", bytes.NewReader(data[:5]))
	if err != nil {
		t.Fatalf("req1: %v", err)
	}
	req1.Header.Set("Content-Range", "bytes 0-4/10")
	resp, err := client.Do(req1)
	if err != nil {
		t.Fatalf("upload1: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload1 status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// check bitmap after first chunk
	respBM, err := client.Get(ts.URL + "/bitmap/s1")
	if err != nil {
		t.Fatalf("bitmap: %v", err)
	}
	bm, err := io.ReadAll(respBM.Body)
	respBM.Body.Close()
	if err != nil {
		t.Fatalf("read bitmap: %v", err)
	}
	if len(bm) != 1 || bm[0]&0x1 == 0 {
		t.Fatalf("unexpected bitmap: %v", bm)
	}

	// upload second chunk
	req2, err := http.NewRequest("PUT", ts.URL+"/upload/s1", bytes.NewReader(data[5:]))
	if err != nil {
		t.Fatalf("req2: %v", err)
	}
	req2.Header.Set("Content-Range", "bytes 5-9/10")
	resp, err = client.Do(req2)
	if err != nil {
		t.Fatalf("upload2: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload2 status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// parallel range downloads
	var wg sync.WaitGroup
	parts := make([][]byte, 2)
	ranges := [][2]int{{0, 4}, {5, 9}}
	for i, r := range ranges {
		wg.Add(1)
		go func(i int, r [2]int) {
			defer wg.Done()
			req, err := http.NewRequest("GET", ts.URL+"/download/s1", nil)
			if err != nil {
				t.Errorf("req%d: %v", i, err)
				return
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", r[0], r[1]))
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("get%d: %v", i, err)
				return
			}
			if resp.StatusCode != http.StatusPartialContent {
				t.Errorf("status%d: %d", i, resp.StatusCode)
			}
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Errorf("read%d: %v", i, err)
				return
			}
			parts[i] = b
		}(i, r)
	}
	wg.Wait()

	got := append(parts[0], parts[1]...)
	if string(got) != string(data) {
		t.Fatalf("download mismatch: got %q want %q", got, data)
	}
}
