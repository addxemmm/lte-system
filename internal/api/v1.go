// v1 implements the standard REST API (/api/v1/*). See docs/API.md and
// docs/api/openapi.yaml for the contract.
//
// Every endpoint uses proper HTTP status codes, the
// {"code","message","data","request_id"} envelope, per-field 422 details,
// no silent fallbacks (unknown band is an error), and idempotent stop.
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
	"github.com/addxemmm/lte-system/internal/sdr"
	"github.com/addxemmm/lte-system/internal/sim"
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
	{http.MethodGet, "/api/v1/network", "effective read-only UE network plan"},
	{http.MethodGet, "/api/v1/ues", "current structured UE session snapshot"},
	{http.MethodGet, "/api/v1/ues/", "current structured UE session detail (prefix)"},
	{http.MethodGet, "/api/v1/diagnostics/connectivity", "read-only connectivity evidence"},
	{http.MethodPost, "/api/v1/crack/jobs", "start APN password cracking"},
	{http.MethodGet, "/api/v1/crack/result", "cracking result"},
	{http.MethodGet, "/api/v1/subscribers", "list authorized subscribers"},
	{http.MethodPost, "/api/v1/subscribers", "create authorized subscriber"},
	{http.MethodGet, "/api/v1/subscribers/", "authorized subscriber detail (prefix)"},
	{http.MethodPatch, "/api/v1/subscribers/", "update subscriber metadata (prefix)"},
	{http.MethodDelete, "/api/v1/subscribers/", "delete authorized subscriber (prefix)"},
	{http.MethodPost, "/api/v1/config/subscribers", "replace validated subscriber database while stopped"},
	{http.MethodPost, "/api/v1/config/wordlist", "upload crack dictionary"},
	{http.MethodGet, "/api/v1/captures/", "download capture by id (prefix)"},
	{http.MethodPost, "/api/v1/simcards", "program a SIM card"},
	{http.MethodGet, "/api/v1/profile", "saved launch profile"},
	{http.MethodGet, "/api/v1/health", "health + SDR detection"},
}

// serveV1 dispatches /api/v1/* with method enforcement and v1 envelopes.
func (s *Server) serveV1(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/v1/ues/") {
		wanted := strings.TrimPrefix(path, "/api/v1/ues/")
		if wanted == "" || strings.Contains(wanted, "/") {
			writeV1(w, r, CodeNotFound, "not found: "+path, nil)
			return
		}
		s.handleV1UE(w, r, wanted)
		return
	}
	if strings.HasPrefix(path, "/api/v1/subscribers/") {
		wanted := strings.TrimPrefix(path, "/api/v1/subscribers/")
		if wanted == "" || strings.Contains(wanted, "/") {
			writeV1(w, r, CodeNotFound, "not found: "+path, nil)
			return
		}
		s.handleV1Subscriber(w, r, wanted)
		return
	}
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
	case path == "/api/v1/network" && r.Method == http.MethodGet:
		s.handleV1Network(w, r)
	case path == "/api/v1/ues":
		s.handleV1UEs(w, r)
	case path == "/api/v1/diagnostics/connectivity" && r.Method == http.MethodGet:
		s.handleV1ConnectivityDiagnostics(w, r)
	case path == "/api/v1/crack/jobs" && r.Method == http.MethodPost:
		s.handleV1CrackStart(w, r)
	case path == "/api/v1/crack/result" && r.Method == http.MethodGet:
		s.handleV1CrackResult(w, r)
	case path == "/api/v1/subscribers":
		s.handleV1Subscribers(w, r)
	case path == "/api/v1/config/subscribers" && r.Method == http.MethodPost:
		s.handleV1SubscriberUpload(w, r)
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
		data["warning"] = "TDD band: uplink EARFCN is pinned explicitly; verify UE sessions with GET /api/v1/ues"
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

// handleV1Network returns only the Manager's effective configuration snapshot.
// Rule presence and plan activation are not evidence of external reachability.
func (s *Server) handleV1Network(w http.ResponseWriter, r *http.Request) {
	writeV1(w, r, CodeOK, "ok", s.mgr.NetworkPlanSnapshot())
}

// ---- POST /api/v1/crack/jobs ----

func (s *Server) handleV1CrackStart(w http.ResponseWriter, r *http.Request) {
	if crack.HashcatRunning() {
		writeV1(w, r, CodeConflict, "a crack job is already running", nil)
		return
	}
	_, hash, err := crack.ExtractCHAP(r.Context(), s.cfg)
	if err != nil || hash == "" {
		s.writeV1CHAPFailure(w, r)
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
	username, hash, err := crack.ExtractCHAP(r.Context(), s.cfg)
	if err != nil || hash == "" {
		s.writeV1CHAPFailure(w, r)
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
		"state": "ready", "username": username, "password": password,
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

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.mgr.LoadProfile()
	out := map[string]any{"has_profile": ok}
	if ok {
		out["profile"] = p
	}
	writeV1(w, r, CodeOK, "ok", out)
}

func (s *Server) handleV1Capture(w http.ResponseWriter, r *http.Request, id string) {
	name, ok := map[string]string{
		"lte-data": s.cfg.PcapLTEData,
		"s1ap":     s.cfg.PcapS1AP,
		"enb":      s.cfg.PcapENB,
		"epc":      s.cfg.PcapEPC,
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
	var id int
	var msg string
	err := s.mgr.RunWhileStopped(ctx, func() error {
		var programErr error
		id, msg, programErr = sim.Program(ctx, s.cfg, req)
		return programErr
	})
	if errors.Is(err, lte.ErrCellMustBeStopped) {
		writeV1(w, r, CodeConflict, "cell must be stopped before programming a SIM", nil)
		return
	}
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
