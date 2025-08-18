package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"lvmsync_go/common"
	"lvmsync_go/internal/config"
	digestpkg "lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsyncwire"
)

const rsyncMaxFrame = 1 << 20

type writeOnlyReadWriter struct{ io.Writer }

func (w writeOnlyReadWriter) Read([]byte) (int, error) { return 0, io.EOF }

// streamRsyncDelta performs a byte-level delta pre-pass using rsyncwire.
// It streams signatures and deltas followed by a digest frame.
// When out implements io.Closer, it is closed on completion.
func (t *Transfer) streamRsyncDelta(ctx context.Context, cfg *config.Config, snapshot, origin string, out io.Writer) (err error) {
	snap, err := os.Open(snapshot)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer common.CloseWithErr(snap, &err, "close snapshot")

	orig, err := os.Open(origin)
	if err != nil {
		return fmt.Errorf("open origin: %w", err)
	}
	defer common.CloseWithErr(orig, &err, "close origin")

	rw := writeOnlyReadWriter{out}
	cl := rsyncwire.NewClient(rsyncwire.NewStream(rw, rsyncMaxFrame))
	if _, err := cl.SendSignatures(orig); err != nil {
		return fmt.Errorf("send signatures: %w", err)
	}
	if _, err := orig.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek origin: %w", err)
	}

	const chunk = 32 * 1024
	bufSnap := make([]byte, chunk)
	bufOrig := make([]byte, chunk)
	var off int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		nSnap, errSnap := snap.Read(bufSnap)
		nOrig, errOrig := orig.Read(bufOrig)
		n := nSnap
		if nOrig < n {
			n = nOrig
		}
		if n > 0 {
			i := 0
			for i < n {
				if bufSnap[i] != bufOrig[i] {
					start := i
					for i < n && bufSnap[i] != bufOrig[i] {
						i++
					}
					if err := cl.SendDelta(off+int64(start), bufSnap[start:i]); err != nil {
						return fmt.Errorf("send delta: %w", err)
					}
				} else {
					i++
				}
			}
			off += int64(n)
		}
		if errSnap == io.EOF || errOrig == io.EOF {
			break
		}
		if errSnap != nil {
			return fmt.Errorf("read snapshot: %w", errSnap)
		}
		if errOrig != nil {
			return fmt.Errorf("read origin: %w", errOrig)
		}
	}

	sum, err := digestpkg.SumFile(snapshot, cfg.ChecksumAlgorithm)
	if err != nil {
		return fmt.Errorf("compute digest: %w", err)
	}
	if err := cl.SendDigest(cfg.ChecksumAlgorithm, sum); err != nil {
		return fmt.Errorf("send digest: %w", err)
	}

	if closer, ok := out.(io.Closer); ok {
		if err := closer.Close(); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to close output: %w", err)
		}
	}
	return nil
}
