package transfer

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/device"
	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestSaveAndReadResumeState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	cfg := &config.Config{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		ResumeState:       path,
		DedupMode:         "fixed",
		CheckpointBytes:   4,
		ResumeToken:       "tok",
	}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	digest := blake3.Sum256([]byte("data"))
	cfg.FirstBlockDigest = hex.EncodeToString(digest[:])
	saveResumeState(cfg, rt, 0, digest, 4, zap.NewNop())
	wal := path + ".wal"
	if _, err := os.Stat(wal); err != nil {
		t.Fatalf("expected resume wal file: %v", err)
	}
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}, digest)
	if cfg.ResumeToken != "tok" {
		t.Fatalf("resume token not persisted: %s", cfg.ResumeToken)
	}
	rc := cp.chunk("fixed")
	if rc.Offset != 0 || rc.Length != 4 || hex.EncodeToString(rc.Chunk[:]) != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected checkpoint: %+v", rc)
	}
	finalizeResumeState(cfg, rt, zap.NewNop())
	if _, err := os.Stat(wal); !os.IsNotExist(err) {
		t.Fatalf("resume wal not removed: %v", err)
	}
}

func TestReadResumeStateDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	good := blake3.Sum256([]byte("data"))
	cfg := &config.Config{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		ResumeState:       path,
		DedupMode:         "fixed",
		CheckpointBytes:   4,
		FirstBlockDigest:  hex.EncodeToString(good[:]),
	}
	rt := &resumeTracker{}
	saveResumeState(cfg, rt, 0, good, 4, zap.NewNop())
	bad := blake3.Sum256([]byte("other"))
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{FSUUID: cfg.DeviceUUID}, bad)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on digest mismatch")
	}
}

func TestReadResumeStateDedupModeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		ResumeState:       path,
		DedupMode:         "fixed",
		CheckpointBytes:   4,
		FirstBlockDigest:  hex.EncodeToString(dig[:]),
	}
	rt := &resumeTracker{}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cfg.DedupMode = "cdc"
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on dedup mode mismatch")
	}
}

// TestResumeStateWALRecovery verifies that a WAL left behind after a crash is
// preferred over the stale resume state file.
func TestResumeStateWALRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	digest := blake3.Sum256([]byte("data"))

	// Write stale resume state file.
	stale := resumeState{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		DedupMode:         "fixed",
		Fixed: resumeChunkState{
			Offset: 8,
			Length: 4,
			Chunk:  hex.EncodeToString(digest[:]),
		},
	}
	b, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write stale resume: %v", err)
	}

	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 1, FirstBlockDigest: hex.EncodeToString(digest[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, digest, 4, zap.NewNop())

	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}, digest)
	rc := cp.chunk("fixed")
	if rc.Offset != 0 {
		t.Fatalf("expected wal offset 0, got %d", rc.Offset)
	}
}

// TestResumeStateCorruptWALIgnored ensures a corrupt WAL doesn't prevent
// reading the valid resume state file.
func TestResumeStateCorruptWALIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	wal := path + ".wal"

	// Write corrupt WAL.
	if err := os.WriteFile(wal, []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("write wal: %v", err)
	}

	digest := blake3.Sum256([]byte("data"))
	state := resumeState{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		DedupMode:         "fixed",
		Fixed: resumeChunkState{
			Offset: 12,
			Length: 4,
			Chunk:  hex.EncodeToString(digest[:]),
		},
	}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write resume: %v", err)
	}

	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", FirstBlockDigest: hex.EncodeToString(digest[:])}
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{}, digest)
	rc := cp.chunk("fixed")
	if rc.Offset != 12 {
		t.Fatalf("expected offset 12 from resume state, got %d", rc.Offset)
	}
}

func TestReadResumeStateSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 200, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on size mismatch")
	}
}

func TestReadResumeStateFSUUIDMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "other", Major: 1, Minor: 2, ManifestEpoch: 1}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on fs uuid mismatch")
	}
}

func TestReadResumeStateEpochMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 2}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on epoch mismatch")
	}
}

func TestReadResumeStateKernelUUIDMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k2", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on kernel uuid mismatch")
	}
}

func TestReadResumeStateGPTUUIDMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g2", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on gpt uuid mismatch")
	}
}

func TestReadResumeStateMBRMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000002", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on mbr signature mismatch")
	}
}

func TestReadResumeStateMajorMinorMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{id: device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 2, ManifestEpoch: 1}}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 3, Minor: 2, ManifestEpoch: 1}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on major mismatch")
	}
	cp = readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 100, KernelUUID: "k", GPTUUID: "g", MBRSignature: "00000001", FSUUID: "fsuuid", Major: 1, Minor: 4, ManifestEpoch: 1}, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on minor mismatch")
	}
}
