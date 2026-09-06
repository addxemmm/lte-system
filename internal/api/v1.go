// v1 implements the standard REST API (/api/v1/*). See docs/API.md and
// docs/api/openapi.yaml for the contract.
//
// Differences from the frozen legacy routes: proper HTTP status codes,
// {"code","message","data","request_id"} envelope, per-field 422 details,
// no silent fallbacks (unknown band is an error), idempotent stop.
package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/addxemmm/lte-system/internal/crack"
	"github.com/addxemmm/lte-system/internal/lte"
	"github.com/addxemmm/lte-system/internal/parser"
	"github.com/addxemmm/lte-system/internal/sdr"
	"github.com/addxemmm/lte-system/internal/sim"
	"github.com/addxemmm/lte-system/internal/subscriber"
	"github.com/addxemmm/lte-system/internal/sysop"
)

// v1Route documents one endpoint (kept in sync with serveV1 + openapi.yaml).
type v1Route struct {
	method  string
	path    string
	summary string
}

// v1Routes is the full v1 surface. serveV1 dispatches on it; the captures
// entry is a prefix (id follows, e.g. /api/v1/captures/lte-data).
var v1Routes = []v1Route{
	{http.MethodPost, "/api/v1/cell", "start cell"},
	{http.MethodGet, "/api/v1/cell", "cell status"},
	{http.MethodDelete, "/api/v1/cell", "stop cell (idempotent; stopping idle succeeds)"},
	{http.MethodGet, "/api/v1/ue", "attached UE snapshot"},
	{http.MethodPost, "/api/v1/crack/jobs", "start APN password cracking"},
	{http.MethodGet, "/api/v1/crack/result", "cracking result"},
	{http.MethodPost, "/api/v1/config/subscribers", "upload user_db.csv"},
	{http.MethodPost, "/api/v1/config/wordlist", "upload crack dictionary"},
	{http.MethodGet, "/api/v1/captures/", "download capture by id (prefix)"},
	{http.MethodPost, "/api/v1/simcards", "program a SIM card"},
	{http.MethodGet, "/api/v1/profile", "saved launch profile + UE list"},
	{http.MethodGet, "/api/v1/health", "health + SDR detection"},
}

// serveV1 dispatches /api/v1/* with method enforcement and v1 envelopes.
func (s *Server) serveV1(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if _, ok := strings.CutPrefix(path, "/api/v1/captures/"); ok {
		if r.Method != http.MethodGet {
			writeV1(w, r, CodeMethod, "method not allowed, want GET", nil)
			return
		}
		s.handleV1Capture(w, r, strings.TrimPrefix(path, "/api/v1/captures/"))
		return
	}
	known := false
	for _, rt := range v1Routes {
		if rt.path == path {
			known = true
			break
		}
	}
	if !known {
		writeV1(w, r, CodeNotFound, "not found: "+path, nil)
		return
	}
	switch {
	case path == "/api/v1/cell" && r.Method == http.MethodPost:
		s.handleV1Start(w, r)
	case path == "/api/v1/cell" && r.Method == http.MethodGet:
		s.handleV1CellStatus(w, r)
	case path == "/api/v1/cell" && r.Method == http.MethodDelete:
		s.handleV1Stop(w, r)
	case path == "/api/v1/ue" && r.Method == http.MethodGet:
		s.handleV1UE(w, r)
	case path == "/api/v1/crack/jobs" && r.Method == http.MethodPost:
		s.handleV1CrackStart(w, r)
	case path == "/api/v1/crack/result" && r.Method == http.MethodGet:
		s.handleV1CrackResult(w, r)
	case path == "/api/v1/config/subscribers" && r.Method == http.MethodPost:
		s.handleV1Upload(w, r, "userdb", s.cfg.UserDBPath(), "takes effect after restart (EPC reads at boot)")
	case path == "/api/v1/config/wordlist" && r.Method == http.MethodPost:
		s.handleV1Upload(w, r, "wordlist", s.cfg.WordlistPath(), "used by the next crack run")
	case path == "/api/v1/simcards" && r.Method == http.MethodPost:
		s.handleV1SIM(w, r)
	case path == "/api/v1/profile" && r.Method == http.MethodGet:
		s.handleProfile(w, r)
	case path == "/api/v1/health" && r.Method == http.MethodGet:
		s.handleV1Health(w, r)
	default:
		writeV1(w, r, CodeMethod, "method not allowed for "+path, nil)
	}
}

// ---- POST /api/v1/cell ----

func (s *Server) handleV1Start(w http.ResponseWriter, r *http.Request) {
	var p lte.StartParams
	if err := decodeJSONObject(w, r, &p, 1<<20, true); err != nil {
		if errors.Is(err, errJSONTooLarge) {
			writeV1(w, r, CodeTooLarge, "JSON body exceeds limit", map[string]any{"max_bytes": 1 << 20})
			return
		}
		writeV1(w, r, CodeMalformed, "malformed JSON body", nil)
		return
	}
	// Profile inheritance: empty fields fall back to the saved profile.
	s.mgr.OverlayProfile(&p)
	if issues := p.ValidateDetailed(); len(issues) > 0 {
		errs := make([]map[string]string, 0, len(issues))
		for _, is := range issues {
			errs = append(errs, map[string]string{"field": is.Field, "reason": is.Reason})
		}
		writeV1(w, r, CodeInvalid, "validation failed", map[string]any{"errors": errs})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	_, err := s.mgr.Start(ctx, p)
	if err != nil {
		switch {
		case err.Error() == "is running":
			writeV1(w, r, CodeConflict, "cell already running", nil)
		case strings.Contains(err.Error(), "device is not connected"):
			writeV1(w, r, CodeNoHardware, "no SDR device attached", map[string]any{"detail": err.Error()})
		case strings.Contains(err.Error(), "unknown network interface"):
			writeV1(w, r, CodeInvalid, err.Error(), map[string]any{"errors": []map[string]string{{"field": "network", "reason": err.Error()}}})
		default:
			writeV1(w, r, CodeInternal, "start failed: "+err.Error(), nil)
		}
		return
	}
	band, _ := lte.Lookup(p.Band)
	data := map[string]any{"band": p.Band, "apn": p.APN, "net_name": p.FullNetName}
	if band.IsTDD() {
		data["warning"] = "TDD band: uplink EARFCN is pinned explicitly; verify UE attach with GET /api/v1/ue"
	}
	writeV1(w, r, CodeOK, "cell started", data)
}

// ---- GET /api/v1/cell, DELETE /api/v1/cell ----

func (s *Server) handleV1CellStatus(w http.ResponseWriter, r *http.Request) {
	writeV1(w, r, CodeOK, "ok", s.mgr.IsRunning())
}

func (s *Server) handleV1Stop(w http.ResponseWriter, r *http.Request) {
	stopped := s.mgr.Stop()
	state := s.mgr.IsRunning()
	if state.Running || state.Pcap {
		writeV1(w, r, CodeInternal, "stop failed, inspect logs", nil)
		return
	}
	if stopped {
		writeV1(w, r, CodeOK, "cell stopped", map[string]any{"stopped": true})
		return
	}
	writeV1(w, r, CodeOK, "already stopped", map[string]any{"stopped": false})
}

// ---- GET /api/v1/ue ----

func (s *Server) handleV1UE(w http.ResponseWriter, r *http.Request) {
	if !sysop.Running("srsepc") && !sysop.Running("srsenb") {
		writeV1(w, r, CodePrecondition, "cell not running", nil)
		return
	}
	info, err := parser.ParseEPCLog(s.cfg.LogPath(s.cfg.EPCLogName))
	if err != nil || !info.Found {
		writeV1(w, r, CodeNotFound, "no UE attached", nil)
		return
	}
	writeV1(w, r, CodeOK, "ok", map[string]any{
		"apn": nilStr(info.APN), "imsi": nilStr(info.IMSI), "ip": nilStr(info.IP),
	})
}

// ---- POST /api/v1/crack/jobs ----

func (s *Server) handleV1CrackStart(w http.ResponseWriter, r *http.Request) {
	if crack.HashcatRunning() {
		writeV1(w, r, CodeConflict, "a crack job is already running", nil)
		return
	}
	info, err := parser.ParseEPCLog(s.cfg.LogPath(s.cfg.EPCLogName))
	if err != nil || !info.Found {
		writeV1(w, r, CodeNotFound, "no UE data: start the cell and attach a UE first", nil)
		return
	}
	_, hash, err := crack.ExtractCHAP(r.Context(), s.cfg)
	if err != nil || hash == "" {
		writeV1(w, r, CodePrecondition, "no CHAP handshake in capture (UE may not use CHAP)", nil)
		return
	}
	// Cracking needs the cell stopped (frees CPU, freezes the pcap).
	if sysop.Running("srsepc") || sysop.Running("srsenb") {
		if !s.mgr.Stop() && (sysop.Running("srsepc") || sysop.Running("srsenb")) {
			writeV1(w, r, CodeInternal, "could not stop cell, stop it manually and retry", nil)
			return
		}
	}
	if err := crack.StartAsync(s.cfg, hash); err != nil {
		writeV1(w, r, CodeInternal, "could not launch hashcat", nil)
		return
	}
	writeV1Status(w, r, http.StatusAccepted, CodeOK, "crack job running",
		map[string]any{"state": "running", "poll": "/api/v1/crack/result"})
}

// ---- GET /api/v1/crack/result ----

func (s *Server) handleV1CrackResult(w http.ResponseWriter, r *http.Request) {
	if crack.HashcatRunning() {
		writeV1(w, r, CodeOK, "ok", map[string]any{"state": "running"})
		return
	}
	info, err := parser.ParseEPCLog(s.cfg.LogPath(s.cfg.EPCLogName))
	if err != nil || !info.Found {
		writeV1(w, r, CodeNotFound, "no UE data: start the cell and attach a UE first", nil)
		return
	}
	username, hash, err := crack.ExtractCHAP(r.Context(), s.cfg)
	if err != nil || hash == "" {
		writeV1(w, r, CodePrecondition, "no CHAP handshake in capture (UE may not use CHAP)", nil)
		return
	}
	password, err := crack.Show(r.Context(), s.cfg, hash)
	if err != nil {
		writeV1(w, r, CodeInternal, "hashcat --show failed", nil)
		return
	}
	if password == "" {
		writeV1(w, r, CodeNotFound, "password not in dictionary",
			map[string]any{"state": "no_password", "username": username})
		return
	}
	writeV1(w, r, CodeOK, "ok", map[string]any{
		"state": "ready", "apn": nilStr(info.APN), "imsi": nilStr(info.IMSI),
		"ip": nilStr(info.IP), "username": username, "password": password,
	})
}

// ---- POST /api/v1/config/* (uploads) ----

func (s *Server) handleV1Upload(w http.ResponseWriter, r *http.Request, field, dst, note string) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeV1(w, r, CodeTooLarge, "upload exceeds limit", map[string]any{"max_bytes": s.cfg.MaxUploadBytes})
			return
		}
		writeV1(w, r, CodeMalformed, "malformed multipart body", nil)
		return
	}
	f, _, err := r.FormFile(field)
	if err != nil || f == nil {
		writeV1(w, r, CodeInvalid, "missing file field: "+field, map[string]any{
			"errors": []map[string]string{{"field": field, "reason": "required"}}})
		return
	}
	defer f.Close()
	if field == "userdb" {
		subscriber.Mutex.Lock()
		defer subscriber.Mutex.Unlock()
	}
	n, err := saveUpload(f, dst, s.cfg.MaxUploadBytes)
	if errors.Is(err, errUploadEmpty) {
		writeV1(w, r, CodeInvalid, "missing file field: "+field, map[string]any{
			"errors": []map[string]string{{"field": field, "reason": "empty"}}})
		return
	}
	if errors.Is(err, errUploadTooLarge) {
		writeV1(w, r, CodeTooLarge, "upload exceeds limit", map[string]any{"max_bytes": s.cfg.MaxUploadBytes})
		return
	}
	if err != nil {
		writeV1(w, r, CodeInternal, "upload failed", nil)
		return
	}
	writeV1(w, r, CodeOK, "upload success", map[string]any{"bytes": n, "note": note})
}

// ---- GET /api/v1/health ----

func (s *Server) handleV1Health(w http.ResponseWriter, r *http.Request) {
	det := sdr.Detect()
	st := s.mgr.IsRunning()
	writeV1(w, r, CodeOK, "ok", map[string]any{
		"ok":      true,
		"running": st.Running,
		"sdr":     det,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleV1Capture(w http.ResponseWriter, r *http.Request, id string) {
	name, ok := map[string]string{
		"lte-data": s.cfg.PcapLTEData, "0": s.cfg.PcapLTEData,
		"s1ap": s.cfg.PcapS1AP, "1": s.cfg.PcapS1AP,
		"enb": s.cfg.PcapENB, "2": s.cfg.PcapENB,
		"epc": s.cfg.PcapEPC, "3": s.cfg.PcapEPC,
	}[id]
	if !ok || id == "" {
		writeV1(w, r, CodeNotFound, "unknown capture id (want lte-data|s1ap|enb|epc)", nil)
		return
	}
	path := s.cfg.LogPath(name)
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		writeV1(w, r, CodeNotFound, "capture not ready yet", map[string]any{"id": id})
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Header().Set("X-Request-ID", RequestID(r))
	http.ServeFile(w, r, path)
}

// ---- POST /api/v1/simcards ----

func (s *Server) handleV1SIM(w http.ResponseWriter, r *http.Request) {
	var req sim.WriteRequest
	// Unknown fields are ignored (forward compatibility), unlike legacy.
	if err := decodeJSONObject(w, r, &req, 64*1024, false); err != nil {
		if errors.Is(err, errJSONTooLarge) {
			writeV1(w, r, CodeTooLarge, "JSON body exceeds limit", map[string]any{"max_bytes": 64 * 1024})
			return
		}
		writeV1(w, r, CodeMalformed, "malformed JSON body", nil)
		return
	}
	if strings.TrimSpace(req.IMSI) == "" {
		writeV1(w, r, CodeInvalid, "validation failed", map[string]any{
			"errors": []map[string]string{{"field": "imsi", "reason": "required"}}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()
	id, msg, _ := sim.Program(ctx, s.cfg, req)
	switch id {
	case 1:
		writeV1(w, r, CodeOK, "card programmed", map[string]any{"result": "programmed"})
	case 4:
		writeV1(w, r, CodeConflict, "card already in database", map[string]any{"result": "exists"})
	case 2:
		writeV1(w, r, CodeNoHardware, "no card reader attached", nil)
	case 5:
		writeV1(w, r, CodePrecondition, "no card inserted", nil)
	case 3:
		writeV1(w, r, CodeInternal, "card programmed but database update failed", nil)
	case 6:
		writeV1(w, r, CodeInvalid, msg, nil)
	default:
		writeV1(w, r, CodeInternal, "programming failed", nil)
	}
}
