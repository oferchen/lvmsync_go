package transfer

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"lvmsync_go/dedup"
	hashutil "lvmsync_go/hash"
)

func collectHybridDigests(data []byte, fixed, min, avg, max int) ([][32]byte, error) {
	h := dedup.NewHybridChunker(fixed, min, avg, max)
	rdr := bytes.NewReader(data)
	var out [][32]byte
	for {
		c, err := h.NextChunk(rdr)
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

func resumeHybridFromDigest(data []byte, fixed, min, avg, max int, resume [32]byte) ([][32]byte, error) {
	h := dedup.NewHybridChunker(fixed, min, avg, max)
	rdr := bytes.NewReader(data)
	var out [][32]byte
	found := false
	for {
		c, err := h.NextChunk(rdr)
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

func TestResumeHybrid(t *testing.T) {
	fixed := 128
	min, avg, max := 64, 64, 128
	data := bytes.Repeat([]byte("b"), 512)

	all, err := collectHybridDigests(data, fixed, min, avg, max)
	if err != nil {
		t.Fatalf("collect digests: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected at least two chunks, got %d", len(all))
	}

	resume := all[0]
	resumed, err := resumeHybridFromDigest(data, fixed, min, avg, max, resume)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	expected := all[1:]
	if !reflect.DeepEqual(resumed, expected) {
		t.Fatalf("resumed digests %v, expected %v", resumed, expected)
	}
}
