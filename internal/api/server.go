// Package api exposes the standard /api/v1 HTTP API.
package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/lte"
)

// Server wires handlers to a Manager.
type Server struct {
	cfg      config.Config
	mgr      *lte.Manager
	mux      *http.ServeMux
	diagGate chan struct{}
}

// New builds the standard API routes. Removed root-level legacy paths fall
// through to the same machine-readable 404 envelope as every unknown path.
func New(cfg config.Config, mgr *lte.Manager) *Server {
	s := &Server{cfg: cfg, mgr: mgr, mux: http.NewServeMux(), diagGate: make(chan struct{}, 1)}
	s.mux.HandleFunc("/api/", s.serveV1)
	s.mux.HandleFunc("/", s.handleNotFound)
	return s
}

// Handler returns the mux wrapped in the middleware chain
// (recover -> request id + audit log -> optional bearer auth).
func (s *Server) Handler() http.Handler { return chain(s.mux) }

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeV1(w, r, CodeNotFound, "not found: "+r.URL.Path, nil)
}

var (
	errUploadEmpty    = errors.New("empty upload")
	errUploadTooLarge = errors.New("upload exceeds limit")
	uploadRenameMu    sync.Mutex
)

// saveUpload atomically replaces dst from a unique temporary file.
func saveUpload(src io.Reader, dst string, maxBytes int64) (n int64, err error) {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	n, err = io.Copy(tmp, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return n, err
	}
	if err := tmp.Sync(); err != nil {
		return n, err
	}
	closeErr := tmp.Close()
	closed = true
	if closeErr != nil {
		return n, closeErr
	}
	if n == 0 {
		return 0, errUploadEmpty
	}
	if n > maxBytes {
		return n, errUploadTooLarge
	}
	uploadRenameMu.Lock()
	err = os.Rename(tmpName, dst)
	uploadRenameMu.Unlock()
	if err != nil {
		return n, err
	}
	return n, nil
}
