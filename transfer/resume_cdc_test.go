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
	// Use identical parameters for CDC and hybrid to make the first chunk
	// boundary deterministic and consistent across modes.
	min, avg, max := 64, 64, 64
	fixed := 64
	data := bytes.Repeat([]byte("a"), 512)

	cdcAll, err := collectCDCDigests(data, min, avg, max)
	if err != nil {
		t.Fatalf("collect CDC digests: %v", err)
	}
	hybridAll, err := collectHybridDigests(data, fixed, min, avg, max)
	if err != nil {
		t.Fatalf("collect hybrid digests: %v", err)
	}
	if len(cdcAll) < 2 || len(hybridAll) < 2 {
		t.Fatalf("expected at least two chunks per mode")
	}

	// The resume checkpoint should be identical regardless of dedup mode.
	resume := cdcAll[0]
	if resume != hybridAll[0] {
		t.Fatalf("resume digest mismatch: %x vs %x", resume, hybridAll[0])
	}

	resumedCDC, err := resumeCDCFromDigest(data, min, avg, max, resume)
	if err != nil {
		t.Fatalf("resume CDC: %v", err)
	}
	if !reflect.DeepEqual(resumedCDC, cdcAll[1:]) {
		t.Fatalf("CDC resumed digests %v, expected %v", resumedCDC, cdcAll[1:])
	}

	resumedHybrid, err := resumeHybridFromDigest(data, fixed, min, avg, max, resume)
	if err != nil {
		t.Fatalf("resume hybrid: %v", err)
	}
	if !reflect.DeepEqual(resumedHybrid, hybridAll[1:]) {
		t.Fatalf("hybrid resumed digests %v, expected %v", resumedHybrid, hybridAll[1:])
	}
}
