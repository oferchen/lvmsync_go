package device

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	walpkg "lvmsync_go/internal/wal"
)

const (
	walVersion      = 5
	walHeaderV0Size = 8 + 64 + 64 + 64 + 32
	walHeaderV1Size = 8 + 8 + 64 + 64 + 64 + 32
	walHeaderV2Size = 8 + 8 + 4 + 4 + 64 + 64 + 64 + 32
	walHeaderV3Size = 8 + 8 + 8 + 4 + 4 + 64 + 64 + 64 + 32
	walHeaderV4Size = 8 + 8 + 8 + 4 + 4 + 64 + 64 + 4 + 64 + 32
	walHeaderSize   = walHeaderV4Size + 32
)

type walHeader struct {
	Version uint64
	Size    uint64
	Epoch   uint64
	Major   uint32
	Minor   uint32
	Kernel  [64]byte
	GPT     [64]byte
	MBR     [4]byte
	FS      [64]byte
	Part    [32]byte
	MAC     [32]byte
}

type walHeaderV4 struct {
	Version uint64
	Size    uint64
	Epoch   uint64
	Major   uint32
	Minor   uint32
	Kernel  [64]byte
	GPT     [64]byte
	MBR     [4]byte
	FS      [64]byte
	MAC     [32]byte
}

type walHeaderV3 struct {
	Version uint64
	Size    uint64
	Epoch   uint64
	Major   uint32
	Minor   uint32
	Kernel  [64]byte
	GPT     [64]byte
	FS      [64]byte
	MAC     [32]byte
}

type walHeaderV2 struct {
	Version uint64
	Size    uint64
	Major   uint32
	Minor   uint32
	Kernel  [64]byte
	GPT     [64]byte
	FS      [64]byte
	MAC     [32]byte
}

type walHeaderV1 struct {
	Version uint64
	Size    uint64
	Kernel  [64]byte
	GPT     [64]byte
	FS      [64]byte
	MAC     [32]byte
}

type walHeaderV0 struct {
	Size   uint64
	Kernel [64]byte
	GPT    [64]byte
	FS     [64]byte
	MAC    [32]byte
}

// ErrWALMetadataMismatch indicates the WAL header identity does not match the
// provided device identity.
var ErrWALMetadataMismatch = errors.New("wal metadata mismatch")

// WAL wraps walpkg.WAL and tracks block ranges.
type WAL struct {
        *walpkg.WAL
        header walHeader
        ranges []Range
}

// WALDeps provides overridable dependencies for WAL operations.
type WALDeps struct {
        *walpkg.Deps
        openFile func(string, int, os.FileMode) (*os.File, error)
        stat     func(*os.File) (fs.FileInfo, error)
}

// NewWALDeps constructs default WAL dependencies.
func NewWALDeps() *WALDeps {
        return &WALDeps{
                Deps:     walpkg.NewDeps(),
                openFile: func(name string, flag int, perm os.FileMode) (*os.File, error) { return os.OpenFile(name, flag, perm) },
                stat:     func(f *os.File) (fs.FileInfo, error) { return f.Stat() },
        }
}

// NewWALDepsWithSync constructs WAL dependencies with a custom sync function.
func NewWALDepsWithSync(fn func(string) error) *WALDeps {
        return &WALDeps{
                Deps:     walpkg.NewDepsWithSync(fn),
                openFile: func(name string, flag int, perm os.FileMode) (*os.File, error) { return os.OpenFile(name, flag, perm) },
                stat:     func(f *os.File) (fs.FileInfo, error) { return f.Stat() },
        }
}

func walHeaderMAC(h *walHeader) [32]byte {
	var buf [8 + 8 + 8 + 4 + 4 + 64 + 64 + 4 + 64 + 32]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint64(buf[16:24], h.Epoch)
	binary.LittleEndian.PutUint32(buf[24:28], h.Major)
	binary.LittleEndian.PutUint32(buf[28:32], h.Minor)
	copy(buf[32:96], h.Kernel[:])
	copy(buf[96:160], h.GPT[:])
	copy(buf[160:164], h.MBR[:])
	copy(buf[164:228], h.FS[:])
	copy(buf[228:260], h.Part[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV4(h *walHeaderV4) [32]byte {
	var buf [8 + 8 + 8 + 4 + 4 + 64 + 64 + 4 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint64(buf[16:24], h.Epoch)
	binary.LittleEndian.PutUint32(buf[24:28], h.Major)
	binary.LittleEndian.PutUint32(buf[28:32], h.Minor)
	copy(buf[32:96], h.Kernel[:])
	copy(buf[96:160], h.GPT[:])
	copy(buf[160:164], h.MBR[:])
	copy(buf[164:228], h.FS[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV3(h *walHeaderV3) [32]byte {
	var buf [8 + 8 + 8 + 4 + 4 + 64 + 64 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint64(buf[16:24], h.Epoch)
	binary.LittleEndian.PutUint32(buf[24:28], h.Major)
	binary.LittleEndian.PutUint32(buf[28:32], h.Minor)
	copy(buf[32:96], h.Kernel[:])
	copy(buf[96:160], h.GPT[:])
	copy(buf[160:224], h.FS[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV2(h *walHeaderV2) [32]byte {
	var buf [8 + 8 + 4 + 4 + 64 + 64 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	binary.LittleEndian.PutUint32(buf[16:20], h.Major)
	binary.LittleEndian.PutUint32(buf[20:24], h.Minor)
	copy(buf[24:88], h.Kernel[:])
	copy(buf[88:152], h.GPT[:])
	copy(buf[152:], h.FS[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV1(h *walHeaderV1) [32]byte {
	var buf [8 + 8 + 64 + 64 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Version)
	binary.LittleEndian.PutUint64(buf[8:16], h.Size)
	copy(buf[16:80], h.Kernel[:])
	copy(buf[80:144], h.GPT[:])
	copy(buf[144:], h.FS[:])
	return blake3.Sum256(buf[:])
}

func walHeaderMACV0(h *walHeaderV0) [32]byte {
	var buf [8 + 64 + 64 + 64]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.Size)
	copy(buf[8:72], h.Kernel[:])
	copy(buf[72:136], h.GPT[:])
	copy(buf[136:], h.FS[:])
	return blake3.Sum256(buf[:])
}

// OpenWAL opens or creates a WAL at path for the given device identity. The
// logger must be non-nil and is used to report metadata mismatches.
// It verifies metadata on existing WALs and loads recorded ranges.
func OpenWAL(path string, id DeviceIdentity, logger *zap.Logger, deps *WALDeps) (*WAL, error) {
	if deps == nil {
		deps = NewWALDeps()
	}
	f, err := deps.openFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := deps.stat(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	w := &WAL{WAL: walpkg.New(f, deps.Deps)}
	if st.Size() < walHeaderV0Size {
		if st.Size() > 0 {
			if err := f.Truncate(0); err != nil {
				f.Close()
				return nil, err
			}
			if _, err := f.Seek(0, 0); err != nil {
				f.Close()
				return nil, err
			}
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = id.SizeBytes
		hdr.Epoch = id.ManifestEpoch
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		copy(hdr.Kernel[:], []byte(id.KernelUUID))
		copy(hdr.GPT[:], []byte(id.GPTUUID))
		if v, err := strconv.ParseUint(id.MBRSignature, 16, 32); err == nil {
			binary.LittleEndian.PutUint32(hdr.MBR[:], uint32(v))
		}
		copy(hdr.FS[:], []byte(id.FSUUID))
		hdr.Part = id.PartitionHash
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		binary.LittleEndian.PutUint32(buf[24:28], hdr.Major)
		binary.LittleEndian.PutUint32(buf[28:32], hdr.Minor)
		copy(buf[32:96], hdr.Kernel[:])
		copy(buf[96:160], hdr.GPT[:])
		copy(buf[160:164], hdr.MBR[:])
		copy(buf[164:228], hdr.FS[:])
		copy(buf[228:260], hdr.Part[:])
		copy(buf[260:292], hdr.MAC[:])
		if n, err := f.Write(buf[:]); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		if err := deps.SyncDir(filepath.Dir(path)); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		return w, nil
	}

	var vbuf [8]byte
	if _, err := f.ReadAt(vbuf[:], 0); err != nil {
		f.Close()
		return nil, err
	}
	ver := binary.LittleEndian.Uint64(vbuf[:])
	if ver == walVersion && st.Size() >= walHeaderSize {
		var buf [walHeaderSize]byte
		if _, err := f.ReadAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr walHeader
		hdr.Version = binary.LittleEndian.Uint64(buf[0:8])
		hdr.Size = binary.LittleEndian.Uint64(buf[8:16])
		hdr.Epoch = binary.LittleEndian.Uint64(buf[16:24])
		hdr.Major = binary.LittleEndian.Uint32(buf[24:28])
		hdr.Minor = binary.LittleEndian.Uint32(buf[28:32])
		copy(hdr.Kernel[:], buf[32:96])
		copy(hdr.GPT[:], buf[96:160])
		copy(hdr.MBR[:], buf[160:164])
		copy(hdr.FS[:], buf[164:228])
		copy(hdr.Part[:], buf[228:260])
		copy(hdr.MAC[:], buf[260:292])
		if mac := walHeaderMAC(&hdr); mac != hdr.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		idMBR, _ := strconv.ParseUint(id.MBRSignature, 16, 32)
		if hdr.Size != id.SizeBytes || hdr.Epoch != id.ManifestEpoch || hdr.Major != id.Major || hdr.Minor != id.Minor ||
			string(hdr.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			binary.LittleEndian.Uint32(hdr.MBR[:]) != uint32(idMBR) ||
			string(hdr.FS[:len(id.FSUUID)]) != id.FSUUID ||
			!bytes.Equal(hdr.Part[:], id.PartitionHash[:]) {
			f.Close()
			hdrID := DeviceIdentity{
				SizeBytes:     hdr.Size,
				KernelUUID:    strings.TrimRight(string(hdr.Kernel[:]), "\x00"),
				GPTUUID:       strings.TrimRight(string(hdr.GPT[:]), "\x00"),
				MBRSignature:  fmt.Sprintf("%08x", binary.LittleEndian.Uint32(hdr.MBR[:])),
				FSUUID:        strings.TrimRight(string(hdr.FS[:]), "\x00"),
				PartitionHash: hdr.Part,
				Major:         hdr.Major,
				Minor:         hdr.Minor,
				ManifestEpoch: hdr.Epoch,
			}
			err := fmt.Errorf("precondition: %w", ErrWALMetadataMismatch)
			logger.Error("wal metadata mismatch",
				zap.Uint64("header_size_bytes", hdrID.SizeBytes),
				zap.Uint64("header_manifest_epoch", hdrID.ManifestEpoch),
				zap.Uint32("header_major", hdrID.Major),
				zap.Uint32("header_minor", hdrID.Minor),
				zap.String("header_kernel_uuid", hdrID.KernelUUID),
				zap.String("header_gpt_uuid", hdrID.GPTUUID),
				zap.String("header_mbr_signature", hdrID.MBRSignature),
				zap.String("header_fs_uuid", hdrID.FSUUID),
				zap.String("header_partition_hash", hex.EncodeToString(hdrID.PartitionHash[:])),
				zap.Uint64("identity_size_bytes", id.SizeBytes),
				zap.Uint64("identity_manifest_epoch", id.ManifestEpoch),
				zap.Uint32("identity_major", id.Major),
				zap.Uint32("identity_minor", id.Minor),
				zap.String("identity_kernel_uuid", id.KernelUUID),
				zap.String("identity_gpt_uuid", id.GPTUUID),
				zap.String("identity_mbr_signature", id.MBRSignature),
				zap.String("identity_fs_uuid", id.FSUUID),
				zap.String("identity_partition_hash", hex.EncodeToString(id.PartitionHash[:])),
				zap.Error(err),
			)
			return nil, err
		}
		w.header = hdr
		off := int64(walHeaderSize)
		entry := make([]byte, 16)
		for {
			n, err := f.ReadAt(entry, off)
			if err != nil || n != 16 {
				break
			}
			start := binary.LittleEndian.Uint64(entry[0:8])
			end := binary.LittleEndian.Uint64(entry[8:16])
			w.ranges = append(w.ranges, Range{Start: start, End: end})
			off += 16
		}
		if _, err := f.Seek(off, 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	if ver == walVersion-1 && st.Size() >= walHeaderV4Size {
		var buf4 [walHeaderV4Size]byte
		if _, err := f.ReadAt(buf4[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr4 walHeaderV4
		hdr4.Version = binary.LittleEndian.Uint64(buf4[0:8])
		hdr4.Size = binary.LittleEndian.Uint64(buf4[8:16])
		hdr4.Epoch = binary.LittleEndian.Uint64(buf4[16:24])
		hdr4.Major = binary.LittleEndian.Uint32(buf4[24:28])
		hdr4.Minor = binary.LittleEndian.Uint32(buf4[28:32])
		copy(hdr4.Kernel[:], buf4[32:96])
		copy(hdr4.GPT[:], buf4[96:160])
		copy(hdr4.MBR[:], buf4[160:164])
		copy(hdr4.FS[:], buf4[164:228])
		copy(hdr4.MAC[:], buf4[228:260])
		if mac := walHeaderMACV4(&hdr4); mac != hdr4.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		idMBR, _ := strconv.ParseUint(id.MBRSignature, 16, 32)
		if hdr4.Size != id.SizeBytes || hdr4.Epoch != id.ManifestEpoch || hdr4.Major != id.Major || hdr4.Minor != id.Minor ||
			string(hdr4.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr4.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			binary.LittleEndian.Uint32(hdr4.MBR[:]) != uint32(idMBR) ||
			string(hdr4.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV4Size)
		if _, err := f.ReadAt(data, walHeaderV4Size); err != nil && err != io.EOF {
			f.Close()
			return nil, err
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr4.Size
		hdr.Epoch = hdr4.Epoch
		hdr.Major = hdr4.Major
		hdr.Minor = hdr4.Minor
		hdr.Kernel = hdr4.Kernel
		hdr.GPT = hdr4.GPT
		hdr.MBR = hdr4.MBR
		hdr.FS = hdr4.FS
		hdr.Part = id.PartitionHash
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		binary.LittleEndian.PutUint32(buf[24:28], hdr.Major)
		binary.LittleEndian.PutUint32(buf[28:32], hdr.Minor)
		copy(buf[32:96], hdr.Kernel[:])
		copy(buf[96:160], hdr.GPT[:])
		copy(buf[160:164], hdr.MBR[:])
		copy(buf[164:228], hdr.FS[:])
		copy(buf[228:260], hdr.Part[:])
		copy(buf[260:292], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, err
			} else if n != len(data) {
				f.Close()
				return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	if ver == walVersion-2 && st.Size() >= walHeaderV3Size {
		var buf3 [walHeaderV3Size]byte
		if _, err := f.ReadAt(buf3[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr3 walHeaderV3
		hdr3.Version = binary.LittleEndian.Uint64(buf3[0:8])
		hdr3.Size = binary.LittleEndian.Uint64(buf3[8:16])
		hdr3.Epoch = binary.LittleEndian.Uint64(buf3[16:24])
		hdr3.Major = binary.LittleEndian.Uint32(buf3[24:28])
		hdr3.Minor = binary.LittleEndian.Uint32(buf3[28:32])
		copy(hdr3.Kernel[:], buf3[32:96])
		copy(hdr3.GPT[:], buf3[96:160])
		copy(hdr3.FS[:], buf3[160:224])
		copy(hdr3.MAC[:], buf3[224:256])
		if mac := walHeaderMACV3(&hdr3); mac != hdr3.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr3.Size != id.SizeBytes || hdr3.Epoch != id.ManifestEpoch || hdr3.Major != id.Major || hdr3.Minor != id.Minor ||
			string(hdr3.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr3.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			string(hdr3.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV3Size)
		if _, err := f.ReadAt(data, walHeaderV3Size); err != nil && err != io.EOF {
			f.Close()
			return nil, err
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr3.Size
		hdr.Epoch = hdr3.Epoch
		hdr.Major = hdr3.Major
		hdr.Minor = hdr3.Minor
		hdr.Kernel = hdr3.Kernel
		hdr.GPT = hdr3.GPT
		if v, err := strconv.ParseUint(id.MBRSignature, 16, 32); err == nil {
			binary.LittleEndian.PutUint32(hdr.MBR[:], uint32(v))
		}
		hdr.FS = hdr3.FS
		hdr.Part = id.PartitionHash
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		binary.LittleEndian.PutUint32(buf[24:28], hdr.Major)
		binary.LittleEndian.PutUint32(buf[28:32], hdr.Minor)
		copy(buf[32:96], hdr.Kernel[:])
		copy(buf[96:160], hdr.GPT[:])
		copy(buf[160:164], hdr.MBR[:])
		copy(buf[164:228], hdr.FS[:])
		copy(buf[228:260], hdr.Part[:])
		copy(buf[260:292], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, err
			} else if n != len(data) {
				f.Close()
				return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		w.ranges = []Range{}
		for off := 0; off+16 <= len(data); off += 16 {
			start := binary.LittleEndian.Uint64(data[off : off+8])
			end := binary.LittleEndian.Uint64(data[off+8 : off+16])
			w.ranges = append(w.ranges, Range{Start: start, End: end})
		}
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	if ver == walVersion-3 && st.Size() >= walHeaderV2Size {
		var buf2 [walHeaderV2Size]byte
		if _, err := f.ReadAt(buf2[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr2 walHeaderV2
		hdr2.Version = binary.LittleEndian.Uint64(buf2[0:8])
		hdr2.Size = binary.LittleEndian.Uint64(buf2[8:16])
		hdr2.Major = binary.LittleEndian.Uint32(buf2[16:20])
		hdr2.Minor = binary.LittleEndian.Uint32(buf2[20:24])
		copy(hdr2.Kernel[:], buf2[24:88])
		copy(hdr2.GPT[:], buf2[88:152])
		copy(hdr2.FS[:], buf2[152:216])
		copy(hdr2.MAC[:], buf2[216:248])
		if mac := walHeaderMACV2(&hdr2); mac != hdr2.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr2.Size != id.SizeBytes || hdr2.Major != id.Major || hdr2.Minor != id.Minor ||
			string(hdr2.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr2.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			string(hdr2.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV2Size)
		if _, err := f.ReadAt(data, walHeaderV2Size); err != nil && err != io.EOF {
			f.Close()
			return nil, err
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr2.Size
		hdr.Epoch = id.ManifestEpoch
		hdr.Major = hdr2.Major
		hdr.Minor = hdr2.Minor
		hdr.Kernel = hdr2.Kernel
		hdr.GPT = hdr2.GPT
		if v, err := strconv.ParseUint(id.MBRSignature, 16, 32); err == nil {
			binary.LittleEndian.PutUint32(hdr.MBR[:], uint32(v))
		}
		hdr.FS = hdr2.FS
		hdr.Part = id.PartitionHash
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		binary.LittleEndian.PutUint32(buf[24:28], hdr.Major)
		binary.LittleEndian.PutUint32(buf[28:32], hdr.Minor)
		copy(buf[32:96], hdr.Kernel[:])
		copy(buf[96:160], hdr.GPT[:])
		copy(buf[160:164], hdr.MBR[:])
		copy(buf[164:228], hdr.FS[:])
		copy(buf[228:260], hdr.Part[:])
		copy(buf[260:292], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, err
			} else if n != len(data) {
				f.Close()
				return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		w.ranges = []Range{}
		for off := 0; off+16 <= len(data); off += 16 {
			start := binary.LittleEndian.Uint64(data[off : off+8])
			end := binary.LittleEndian.Uint64(data[off+8 : off+16])
			w.ranges = append(w.ranges, Range{Start: start, End: end})
		}
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	if ver == walVersion-4 && st.Size() >= walHeaderV1Size {
		var buf1 [walHeaderV1Size]byte
		if _, err := f.ReadAt(buf1[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr1 walHeaderV1
		hdr1.Version = binary.LittleEndian.Uint64(buf1[0:8])
		hdr1.Size = binary.LittleEndian.Uint64(buf1[8:16])
		copy(hdr1.Kernel[:], buf1[16:80])
		copy(hdr1.GPT[:], buf1[80:144])
		copy(hdr1.FS[:], buf1[144:208])
		copy(hdr1.MAC[:], buf1[208:240])
		if mac := walHeaderMACV1(&hdr1); mac != hdr1.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr1.Size != id.SizeBytes ||
			string(hdr1.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr1.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			string(hdr1.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV1Size)
		if _, err := f.ReadAt(data, walHeaderV1Size); err != nil && err != io.EOF {
			f.Close()
			return nil, err
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr1.Size
		hdr.Epoch = id.ManifestEpoch
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		hdr.Kernel = hdr1.Kernel
		hdr.GPT = hdr1.GPT
		if v, err := strconv.ParseUint(id.MBRSignature, 16, 32); err == nil {
			binary.LittleEndian.PutUint32(hdr.MBR[:], uint32(v))
		}
		hdr.FS = hdr1.FS
		hdr.Part = id.PartitionHash
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		binary.LittleEndian.PutUint32(buf[24:28], hdr.Major)
		binary.LittleEndian.PutUint32(buf[28:32], hdr.Minor)
		copy(buf[32:96], hdr.Kernel[:])
		copy(buf[96:160], hdr.GPT[:])
		copy(buf[160:164], hdr.MBR[:])
		copy(buf[164:228], hdr.FS[:])
		copy(buf[228:260], hdr.Part[:])
		copy(buf[260:292], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, err
			} else if n != len(data) {
				f.Close()
				return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		w.ranges = []Range{}
		for off := 0; off+16 <= len(data); off += 16 {
			start := binary.LittleEndian.Uint64(data[off : off+8])
			end := binary.LittleEndian.Uint64(data[off+8 : off+16])
			w.ranges = append(w.ranges, Range{Start: start, End: end})
		}
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	if st.Size() >= walHeaderV0Size {
		var buf0 [walHeaderV0Size]byte
		if _, err := f.ReadAt(buf0[:], 0); err != nil {
			f.Close()
			return nil, err
		}
		var hdr0 walHeaderV0
		hdr0.Size = binary.LittleEndian.Uint64(buf0[0:8])
		copy(hdr0.Kernel[:], buf0[8:72])
		copy(hdr0.GPT[:], buf0[72:136])
		copy(hdr0.FS[:], buf0[136:200])
		copy(hdr0.MAC[:], buf0[200:])
		if mac := walHeaderMACV0(&hdr0); mac != hdr0.MAC {
			f.Close()
			return nil, fmt.Errorf("wal: header mac mismatch")
		}
		if hdr0.Size != id.SizeBytes ||
			string(hdr0.Kernel[:len(id.KernelUUID)]) != id.KernelUUID ||
			string(hdr0.GPT[:len(id.GPTUUID)]) != id.GPTUUID ||
			string(hdr0.FS[:len(id.FSUUID)]) != id.FSUUID {
			f.Close()
			return nil, fmt.Errorf("wal: metadata mismatch")
		}
		data := make([]byte, st.Size()-walHeaderV0Size)
		if _, err := f.ReadAt(data, walHeaderV0Size); err != nil && err != io.EOF {
			f.Close()
			return nil, err
		}
		ranges := []Range{}
		for off := 0; off+16 <= len(data); off += 16 {
			start := binary.LittleEndian.Uint64(data[off : off+8])
			end := binary.LittleEndian.Uint64(data[off+8 : off+16])
			ranges = append(ranges, Range{Start: start, End: end})
		}
		var hdr walHeader
		hdr.Version = walVersion
		hdr.Size = hdr0.Size
		hdr.Epoch = id.ManifestEpoch
		hdr.Major = id.Major
		hdr.Minor = id.Minor
		hdr.Kernel = hdr0.Kernel
		hdr.GPT = hdr0.GPT
		if v, err := strconv.ParseUint(id.MBRSignature, 16, 32); err == nil {
			binary.LittleEndian.PutUint32(hdr.MBR[:], uint32(v))
		}
		hdr.FS = hdr0.FS
		hdr.Part = id.PartitionHash
		hdr.MAC = walHeaderMAC(&hdr)
		var buf [walHeaderSize]byte
		binary.LittleEndian.PutUint64(buf[0:8], hdr.Version)
		binary.LittleEndian.PutUint64(buf[8:16], hdr.Size)
		binary.LittleEndian.PutUint64(buf[16:24], hdr.Epoch)
		binary.LittleEndian.PutUint32(buf[24:28], hdr.Major)
		binary.LittleEndian.PutUint32(buf[28:32], hdr.Minor)
		copy(buf[32:96], hdr.Kernel[:])
		copy(buf[96:160], hdr.GPT[:])
		copy(buf[160:164], hdr.MBR[:])
		copy(buf[164:228], hdr.FS[:])
		copy(buf[228:260], hdr.Part[:])
		copy(buf[260:292], hdr.MAC[:])
		if n, err := f.WriteAt(buf[:], 0); err != nil {
			f.Close()
			return nil, err
		} else if n != len(buf) {
			f.Close()
			return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
		}
		if len(data) > 0 {
			if n, err := f.WriteAt(data, walHeaderSize); err != nil {
				f.Close()
				return nil, err
			} else if n != len(data) {
				f.Close()
				return nil, fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(data))
			}
		}
		if err := f.Truncate(int64(walHeaderSize + len(data))); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, err
		}
		w.header = hdr
		w.ranges = ranges
		if _, err := f.Seek(int64(walHeaderSize+len(data)), 0); err != nil {
			f.Close()
			return nil, err
		}
		return w, nil
	}

	f.Close()
	return nil, fmt.Errorf("wal: file too small")
}

// Append records the range using a temporary file for crash safety. The range is
// written to `<path>.tmp`, fsynced, atomically renamed to the WAL path, and the
// directory is fsynced before returning.
func (w *WAL) Append(r Range) error {
	if err := w.WAL.Append(r); err != nil {
		return err
	}
	w.ranges = append(w.ranges, r)
	return nil
}

// Ranges returns the ranges recorded in the WAL.
func (w *WAL) Ranges() []Range { return append([]Range(nil), w.ranges...) }

// Has reports whether the provided range is fully contained in the WAL.
func (w *WAL) Has(start, end uint64) bool {
	for _, r := range w.ranges {
		if start >= r.Start && end <= r.End {
			return true
		}
	}
	return false
}

// Close flushes and closes the underlying WAL.
func (w *WAL) Close() error {
        return w.WAL.Close()
}
