package transfer

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"lvmsync_go/dedup"
	hashutil "lvmsync_go/hash"
)

func collectCDCDigests(data []byte, min, avg, max int) ([][32]byte, error) {
	ch := dedup.NewChunker(min, avg, max)
	rdr := bytes.NewReader(data)
	var out [][32]byte
	for {
		c, err := ch.NextChunk(rdr)
		if err == io.EOF && c.Length == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		out = append(out, hashutil.SumBLAKE3(c.Data))
		if err == io.EOF {
			break
		}
	}
	return out, nil
}

func resumeCDCFromDigest(data []byte, min, avg, max int, resume [32]byte) ([][32]byte, error) {
	ch := dedup.NewChunker(min, avg, max)
	rdr := bytes.NewReader(data)
	var out [][32]byte
	found := false
	for {
		c, err := ch.NextChunk(rdr)
		if err == io.EOF && c.Length == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		digest := hashutil.SumBLAKE3(c.Data)
		if !found {
			if digest == resume {
				found = true
			}
		} else {
			out = append(out, digest)
		}
		if err == io.EOF {
			break
		}
	}
	if !found {
		return nil, io.EOF
	}
	return out, nil
}

func TestResumeCDC(t *testing.T) {
	// Use small CDC parameters for deterministic chunking.
	min, avg, max := 64, 64, 128
	data := bytes.Repeat([]byte("a"), 512)

	all, err := collectCDCDigests(data, min, avg, max)
	if err != nil {
		t.Fatalf("collect digests: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected at least two chunks, got %d", len(all))
	}

	resume := all[0]
	resumed, err := resumeCDCFromDigest(data, min, avg, max, resume)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	expected := all[1:]
	if !reflect.DeepEqual(resumed, expected) {
		t.Fatalf("resumed digests %v, expected %v", resumed, expected)
	}
}
