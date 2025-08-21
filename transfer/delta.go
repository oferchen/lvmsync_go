package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/dedup"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	digestpkg "lvmsync_go/internal/digest"
	"lvmsync_go/internal/privilege"
	"lvmsync_go/internal/rsyncwire"
	manifestpkg "lvmsync_go/manifest"
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

	t.Logger.Warn("plaintext_connection", zap.String("transport", "rsync"), zap.String("docs", "docs/transports.md"))

	rw := writeOnlyReadWriter{out}
	cl := rsyncwire.NewClient(rsyncwire.NewStream(rw, rsyncMaxFrame))
	// Send destination identity to allow early mismatch detection.
	dev, err := device.Detect(ctx, origin, true, "", "", "", "", 0, 0, privilege.New(ctx, t.Logger), t.Logger, device.NewRunner())
	if err != nil {
		return fmt.Errorf("detect origin: %w", err)
	}
	defer dev.Close()
	id, err := dev.Identity(ctx)
	if err != nil {
		return fmt.Errorf("destination identity: %w", err)
	}
	if id.KernelUUID == "" {
		id.KernelUUID = "0"
	}
	if id.GPTUUID == "" {
		id.GPTUUID = "0"
	}
	if id.MBRSignature == "" {
		id.MBRSignature = "0"
	}
	if id.FSUUID == "" {
		id.FSUUID = "0"
	}
	if err := cl.SendIdentity(ctx, id); err != nil {
		return fmt.Errorf("send identity: %w", err)
	}
	if _, err := cl.SendSignatures(ctx, orig); err != nil {
		return fmt.Errorf("send signatures: %w", err)
	}
	if _, err := orig.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek origin: %w", err)
	}

	// Load CDC chunk digests from the manifest when available.
	var digests map[[32]byte]struct{}
	if cfg.ManifestPath != "" {
		idx, err := manifestpkg.Open(cfg.ManifestPath)
		if err != nil {
			return fmt.Errorf("open manifest: %w", err)
		}
		defer idx.Close()
		chunks := idx.CDCChunks()
		digests = make(map[[32]byte]struct{}, len(chunks))
		for _, ch := range chunks {
			digests[ch.Digest] = struct{}{}
		}
	}

	if len(digests) > 0 {
		ch, err := dedup.NewChunker(cfg.CDCMin, cfg.CDCAvg, cfg.CDCMax, cfg.ChunkSeed)
		if err != nil {
			return fmt.Errorf("new chunker: %w", err)
		}
		var off int64
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			c, err := ch.NextChunk(snap)
			if err == io.EOF && c.Length == 0 {
				break
			}
			if err != nil && err != io.EOF {
				return fmt.Errorf("chunk snapshot: %w", err)
			}
			sum := blake3.Sum256(c.Data[:c.Length])
			if _, ok := digests[sum]; !ok {
				if err := cl.SendDelta(ctx, off, c.Data[:c.Length]); err != nil {
					return fmt.Errorf("send delta: %w", err)
				}
			}
			off += int64(c.Length)
			if err == io.EOF {
				break
			}
		}
	} else {
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
						if err := cl.SendDelta(ctx, off+int64(start), bufSnap[start:i]); err != nil {
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
	}

	sum, err := digestpkg.SumFile(snapshot, cfg.ChecksumAlgorithm)
	if err != nil {
		return fmt.Errorf("compute digest: %w", err)
	}
	if err := cl.SendDigest(ctx, cfg.ChecksumAlgorithm, sum); err != nil {
		return fmt.Errorf("send digest: %w", err)
	}

	if closer, ok := out.(io.Closer); ok {
		if err := closer.Close(); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to close output: %w", err)
		}
	}
	return nil
}
