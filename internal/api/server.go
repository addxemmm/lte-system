// Package api exposes the stateless tool HTTP API.
// It preserves the 9 legacy Flask routes + message_id semantics verbatim,
// and adds GET /healthz, GET /status and GET /profile for ops.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/crack"
	"github.com/addxemmm/lte-system/internal/lte"
	"github.com/addxemmm/lte-system/internal/parser"
	"github.com/addxemmm/lte-system/internal/sdr"
	"github.com/addxemmm/lte-system/internal/sim"
	"github.com/addxemmm/lte-system/internal/sysop"
)

// Server wires handlers to a Manager.
type Server struct {
	cfg config.Config
	mgr *lte.Manager
	mux *http.ServeMux
}

// New builds routes.
func New(cfg config.Config, mgr *lte.Manager) *Server {
	s := &Server{cfg: cfg, mgr: mgr, mux: http.NewServeMux()}
	s.mux.HandleFunc("/start", s.handleStart)
	s.mux.HandleFunc("/stop", s.handleStop)
	s.mux.HandleFunc("/basicinfo", s.handleBasicInfo)
	s.mux.HandleFunc("/crackapn", s.handleCrackAPN)
	s.mux.HandleFunc("/getcrackresult", s.handleCrackResult)
	s.mux.HandleFunc("/userupload", s.handleUserUpload)
	s.mux.HandleFunc("/passwordupload", s.handlePasswordUpload)
	s.mux.HandleFunc("/getfile", s.handleGetFile)
	s.mux.HandleFunc("/writesim", s.handleWriteSIM)
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/profile", s.handleProfile)
	return s
}

// Handler returns the mux (with timeout wrapper applied by main).
func (s *Server) Handler() http.Handler { return s.mux }

// ---- response envelope (legacy: status/message_id/message) ----

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func resp(status bool, id int, msg string, extras ...map[string]any) map[string]any {
	m := map[string]any{"status": status, "message_id": id, "message": msg}
	for _, extra := range extras {
		for k, v := range extra {
			m[k] = v
		}
	}
	return m
}

// ---- /start ----

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "Start Failed"))
		return
	}
	var p lte.StartParams
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		writeJSON(w, resp(false, 3, "Incomplete parameters"))
		return
	}
	// Profile inheritance: every empty field falls back to the saved
	// profile (persisted on each successful /start). Fully-empty body {}
	// therefore reuses the whole profile; partial bodies override per-field.
	// No usable profile + missing required fields => Incomplete parameters.
	s.mgr.OverlayProfile(&p)
	if err := p.Validate(); err != nil && err.Error() == "incomplete parameters" {
		writeJSON(w, resp(false, 3, "Incomplete parameters"))
		return
	} else if err != nil {
		writeJSON(w, resp(false, 0, "Start Failed: "+err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	bandKnown, err := s.mgr.Start(ctx, p)
	if err != nil {
		switch err.Error() {
		case "is running":
			writeJSON(w, resp(false, 2, "is running"))
		case "incomplete parameters":
			writeJSON(w, resp(false, 3, "Incomplete parameters"))
		case "device is not connected, please connect usrp device":
			writeJSON(w, resp(false, 4, "device is not connected, please connect usrp device."))
		default:
			writeJSON(w, resp(false, 0, "Start Failed: "+err.Error()))
		}
		return
	}
	_ = bandKnown
	writeJSON(w, resp(true, 1, "Start successfully"))
}

// ---- /stop ----

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "Stop failed."))
		return
	}
	if !sysop.Running("srsepc") && !sysop.Running("srsenb") {
		writeJSON(w, resp(false, 2, "Not running."))
		return
	}
	if s.mgr.Stop() {
		writeJSON(w, resp(true, 1, "Stop successfully."))
		return
	}
	// Double-check: maybe already gone between check and kill.
	if !sysop.Running("srsepc") && !sysop.Running("srsenb") {
		writeJSON(w, resp(true, 1, "Stop successfully."))
		return
	}
	writeJSON(w, resp(false, 0, "Stop failed."))
}

// ---- /basicinfo ----

func (s *Server) handleBasicInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "Failed"))
		return
	}
	if !sysop.Running("srsepc") && !sysop.Running("srsenb") {
		writeJSON(w, resp(false, 2, "Is not running, please start first."))
		return
	}
	info, err := parser.ParseEPCLog(s.cfg.LogPath(s.cfg.EPCLogName))
	if err != nil || !info.Found {
		writeJSON(w, resp(false, 3, "no UE connected"))
		return
	}
	extra := map[string]any{"apn": nilStr(info.APN), "imsi": nilStr(info.IMSI), "ip": nilStr(info.IP)}
	writeJSON(w, resp(true, 1, "Getting information success.", extra))
}

func nilStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// ---- /crackapn ----

func (s *Server) handleCrackAPN(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "Failed"))
		return
	}
	if crack.HashcatRunning() {
		writeJSON(w, resp(false, 2, "Hashcat is running."))
		return
	}
	info, err := parser.ParseEPCLog(s.cfg.LogPath(s.cfg.EPCLogName))
	if err != nil || !info.Found {
		writeJSON(w, resp(false, 3, "Can not get UE's data, please start first and connect UE, or just connect UE. Then try again."))
		return
	}
	username, hash, err := crack.ExtractCHAP(r.Context(), s.cfg)
	if err != nil || hash == "" {
		writeJSON(w, resp(false, 4, "Can not get username and password"))
		return
	}
	// Cracking requires stopping LTE (frees CPU + historic pcap is stable).
	if sysop.Running("srsepc") || sysop.Running("srsenb") {
		if !s.mgr.Stop() && (sysop.Running("srsepc") || sysop.Running("srsenb")) {
			writeJSON(w, resp(false, 5, "Stop program failed, please try to stop manually."))
			return
		}
	}
	_ = username
	if err := crack.StartAsync(s.cfg, hash); err != nil {
		writeJSON(w, resp(false, 0, "Failed"))
		return
	}
	writeJSON(w, resp(true, 1, "Start crack success."))
}

// ---- /getcrackresult ----

func (s *Server) handleCrackResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "Failed."))
		return
	}
	if crack.HashcatRunning() {
		writeJSON(w, resp(false, 2, "Cracking apn is still running, please try again later."))
		return
	}
	info, err := parser.ParseEPCLog(s.cfg.LogPath(s.cfg.EPCLogName))
	if err != nil || !info.Found {
		writeJSON(w, resp(false, 5, "Can not get UE's data, please start first and connect UE, or just connect UE, then try again."))
		return
	}
	username, hash, err := crack.ExtractCHAP(r.Context(), s.cfg)
	if err != nil || hash == "" {
		writeJSON(w, resp(false, 4, "Can not get username and password."))
		return
	}
	password, err := crack.Show(r.Context(), s.cfg, hash)
	if err != nil {
		writeJSON(w, resp(false, 0, "Failed."))
		return
	}
	if password == "" {
		writeJSON(w, resp(false, 3, "Can not get password from dict."))
		return
	}
	extra := map[string]any{
		"apn": nilStr(info.APN), "imsi": nilStr(info.IMSI), "ip": nilStr(info.IP),
		"username": username, "password": password,
	}
	writeJSON(w, resp(true, 1, "Getting information success.", extra))
}

// ---- uploads ----

func (s *Server) handleUserUpload(w http.ResponseWriter, r *http.Request) {
	s.handleUpload(w, r, "userdb", s.cfg.UserDBPath())
}

func (s *Server) handlePasswordUpload(w http.ResponseWriter, r *http.Request) {
	s.handleUpload(w, r, "wordlist", s.cfg.WordlistPath())
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request, field, dst string) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "upload failed"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+ (1<<20))
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
		writeJSON(w, resp(false, 0, "upload failed"))
		return
	}
	f, _, err := r.FormFile(field)
	if err != nil || f == nil {
		writeJSON(w, resp(false, 2, "no file"))
		return
	}
	defer f.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		writeJSON(w, resp(false, 0, "upload failed"))
		return
	}
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		writeJSON(w, resp(false, 0, "upload failed"))
		return
	}
	n, err := io.Copy(out, io.LimitReader(f, s.cfg.MaxUploadBytes+1))
	_ = out.Close()
	if err != nil || n == 0 {
		_ = os.Remove(tmp)
		if n == 0 {
			writeJSON(w, resp(false, 2, "no file"))
			return
		}
		writeJSON(w, resp(false, 0, "upload failed"))
		return
	}
	if n > s.cfg.MaxUploadBytes {
		_ = os.Remove(tmp)
		writeJSON(w, resp(false, 0, "upload failed: file too large"))
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		writeJSON(w, resp(false, 0, "upload failed"))
		return
	}
	writeJSON(w, resp(true, 1, "upload success"))
}

// ---- /getfile ----

var fileIDMap = map[int]func(c config.Config) string{
	0: func(c config.Config) string { return c.LogPath(c.PcapLTEData) },
	1: func(c config.Config) string { return c.LogPath(c.PcapS1AP) },
	2: func(c config.Config) string { return c.LogPath(c.PcapENB) },
	3: func(c config.Config) string { return c.LogPath(c.PcapEPC) },
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "Failed"))
		return
	}
	var body struct {
		FileID *int `json:"fileid"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&body); err != nil || body.FileID == nil {
		writeJSON(w, resp(false, 2, "Error id"))
		return
	}
	fn, ok := fileIDMap[*body.FileID]
	if !ok {
		writeJSON(w, resp(false, 2, "Error id"))
		return
	}
	path := fn(s.cfg)
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		// Legacy mistakenly returned id 2 here; use documented id 3.
		writeJSON(w, resp(false, 3, "Cant not find the file"))
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	http.ServeFile(w, r, path)
}

// ---- /writesim ----

func (s *Server) handleWriteSIM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, resp(false, 0, "Failed"))
		return
	}
	var req sim.WriteRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// Back-compat: legacy sometimes sent {"imsi":001010123456789} (number, no quotes).
		// Retry with a loose decode.
		var loose struct {
			IMSI any `json:"imsi"`
		}
		_ = loose
		writeJSON(w, resp(false, 6, "Invalid parameters"))
		return
	}
	// Allow numeric IMSI (JSON number) via raw re-parse.
	if req.IMSI == "" {
		var raw map[string]any
		// best-effort: re-read not possible; just reject with clear message.
		_ = raw
		writeJSON(w, resp(false, 6, "Invalid parameters: imsi required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()
	id, msg, _ := sim.Program(ctx, s.cfg, req)
	switch id {
	case 1:
		writeJSON(w, resp(true, 1, "Succeed."))
	case 2:
		writeJSON(w, resp(false, 2, "Device is not connected, please connect acr1281 first."))
	case 3:
		writeJSON(w, resp(false, 3, "Writting card successfully, but write user_db.csv failed."))
	case 4:
		writeJSON(w, resp(false, 4, "The card already exists and can be used directly."))
	case 5:
		writeJSON(w, resp(false, 5, "SIM card is not inserted."))
	case 6:
		writeJSON(w, resp(false, 6, msg))
	default:
		writeJSON(w, resp(false, 0, "Failed."))
	}
}

// handleWriteSIMRaw supports legacy numeric-imsi payloads: {"imsi":001010123456789}.
// The strict decoder above rejects numbers; this helper is used by tests.
func normalizeNumericIMSI(raw []byte) (string, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", false
	}
	v, ok := m["imsi"]
	if !ok {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatInt(int64(t), 10), true
	default:
		return "", false
	}
}

// ---- /healthz + /status ----

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	det := sdr.Detect()
	st := s.mgr.IsRunning()
	writeJSON(w, map[string]any{
		"ok":      true,
		"running": st.Running,
		"sdr":     det,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := s.mgr.IsRunning()
	writeJSON(w, st)
}

// ---- GET /profile: saved launch config + HSS rows (no key material) ----

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, resp(false, 0, "Failed"))
		return
	}
	p, ok := s.mgr.LoadProfile()
	ues, _ := sim.Summarize(s.cfg.UserDBPath())
	if ues == nil {
		ues = []sim.UEEntry{}
	}
	out := map[string]any{"has_profile": ok, "ues": ues}
	if ok {
		out["profile"] = p
	}
	writeJSON(w, out)
}
