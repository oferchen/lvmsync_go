package dedup

import (
	"encoding/json"
	"log/syslog"

	"go.uber.org/zap"
)

// ManifestEntry describes a single chunk in the output stream.
type ManifestEntry struct {
	Hash   [32]byte `json:"hash"`
	Offset int64    `json:"offset"`
	Length int      `json:"length"`
}

// Manifest is a list of chunk entries.
type Manifest struct {
	Chunks []ManifestEntry `json:"chunks"`
}

// Append appends a new entry to the manifest.
func (m *Manifest) Append(hash [32]byte, offset int64, length int) {
	m.Chunks = append(m.Chunks, ManifestEntry{Hash: hash, Offset: offset, Length: length})
}

// Marshal returns the JSON representation of the manifest.
func (m *Manifest) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalManifest decodes a manifest from JSON data.
func UnmarshalManifest(b []byte) (Manifest, error) {
	var m Manifest
	err := json.Unmarshal(b, &m)
	return m, err
}

// AuditLog writes the manifest to syslog and the provided zap logger using snake_case fields.
func (m *Manifest) AuditLog(logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	b, err := m.Marshal()
	if err != nil {
		logger.Error("manifest_marshal_error", zap.Error(err))
		_ = logger.Sync()
		return
	}
	if w, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, "lvmsync"); err == nil {
		_, _ = w.Write(b)
		_ = w.Close()
	}
	logger.Info("session_manifest", zap.ByteString("manifest_json", b))
	_ = logger.Sync()
}
