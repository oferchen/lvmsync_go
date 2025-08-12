package h2

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Server handles HTTP/2 transfers with resumable session state.
type Server struct {
	mu       sync.Mutex
	sessions map[string]*Session
	logger   *zap.Logger
}

// NewServer creates an in-memory HTTP/2 server.
func NewServer(logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Server{sessions: make(map[string]*Session), logger: logger}
}

// Handler returns an http.Handler implementing upload, download and bitmap endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/upload/", s.handleUpload)
	mux.HandleFunc("/download/", s.handleDownload)
	mux.HandleFunc("/bitmap/", s.handleBitmap)
	return mux
}

func (s *Server) getSession(id string, total, chunkSize int) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		sess = newSession(id, total, chunkSize)
		s.sessions[id] = sess
	}
	return sess
}

func parseContentRange(h string) (start, end, total int, err error) {
	// format: bytes start-end/total
	if !strings.HasPrefix(h, "bytes ") {
		return 0, 0, 0, fmt.Errorf("invalid content-range")
	}
	parts := strings.Split(strings.TrimPrefix(h, "bytes "), "/")
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid content-range")
	}
	rng := strings.Split(parts[0], "-")
	if len(rng) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid content-range")
	}
	start, err = strconv.Atoi(rng[0])
	if err != nil {
		return
	}
	end, err = strconv.Atoi(rng[1])
	if err != nil {
		return
	}
	total, err = strconv.Atoi(parts[1])
	return
}

func parseRange(h string) (start, end int, err error) {
	// format: bytes=start-end
	if !strings.HasPrefix(h, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range")
	}
	parts := strings.Split(strings.TrimPrefix(h, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range")
	}
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	end, err = strconv.Atoi(parts[1])
	return
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/upload/")
	cr := r.Header.Get("Content-Range")
	start, end, total, err := parseContentRange(cr)
	if err != nil {
		s.logger.Warn("invalid_content_range", zap.String("session_id", id), zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("read_body", zap.String("session_id", id), zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sess := s.getSession(id, total, end-start+1)
	if err := sess.Upload(start, end, b); err != nil {
		s.logger.Warn("upload_failed", zap.String("session_id", id), zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.logger.Info("chunk_uploaded", zap.String("session_id", id), zap.Int("start", start), zap.Int("end", end))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/download/")
	ra := r.Header.Get("Range")
	start, end, err := parseRange(ra)
	if err != nil {
		s.logger.Warn("invalid_range", zap.String("session_id", id), zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	sess := s.sessions[id]
	s.mu.Unlock()
	if sess == nil {
		s.logger.Warn("session_not_found", zap.String("session_id", id))
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	data, err := sess.Download(start, end)
	if err != nil {
		s.logger.Warn("download_failed", zap.String("session_id", id), zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, sess.size))
	w.WriteHeader(http.StatusPartialContent)
	if _, err := w.Write(data); err != nil {
		s.logger.Error("write_body", zap.String("session_id", id), zap.Error(err))
	}
}

func (s *Server) handleBitmap(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/bitmap/")
	s.mu.Lock()
	sess := s.sessions[id]
	s.mu.Unlock()
	if sess == nil {
		s.logger.Warn("session_not_found", zap.String("session_id", id))
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	bm := sess.Bitmap()
	w.Header().Set("Content-Length", strconv.Itoa(len(bm)))
	if _, err := w.Write(bm); err != nil {
		s.logger.Error("write_bitmap", zap.String("session_id", id), zap.Error(err))
	}
}
