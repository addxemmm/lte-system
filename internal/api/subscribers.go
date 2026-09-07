package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/addxemmm/lte-system/internal/lte"
	"github.com/addxemmm/lte-system/internal/subscriber"
)

const subscriberJSONLimit = 64 << 10

type createSubscriberRequest struct {
	Name    string `json:"name"`
	Auth    string `json:"auth"`
	IMSI    string `json:"imsi"`
	Key     string `json:"key"`
	OPType  string `json:"op_type"`
	OP      string `json:"op"`
	OPc     string `json:"opc"`
	AMF     string `json:"amf"`
	SQN     string `json:"sqn"`
	QCI     int    `json:"qci"`
	IPAlloc string `json:"ip_alloc"`
}

type patchSubscriberRequest struct {
	Name    *string         `json:"name"`
	QCI     *int            `json:"qci"`
	IPAlloc *string         `json:"ip_alloc"`
	Auth    json.RawMessage `json:"auth"`
	Key     json.RawMessage `json:"key"`
	OPType  json.RawMessage `json:"op_type"`
	OP      json.RawMessage `json:"op"`
	OPc     json.RawMessage `json:"opc"`
	AMF     json.RawMessage `json:"amf"`
	SQN     json.RawMessage `json:"sqn"`
}

func (s *Server) handleV1Subscribers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleV1SubscriberList(w, r)
	case http.MethodPost:
		s.handleV1SubscriberCreate(w, r)
	default:
		writeV1(w, r, CodeMethod, "method not allowed, want GET or POST", nil)
	}
}

func (s *Server) handleV1Subscriber(w http.ResponseWriter, r *http.Request, wanted string) {
	if !validIMSI(wanted) {
		writeValidation(w, r, "imsi", "must contain exactly 15 digits")
		return
	}
	switch r.Method {
	case http.MethodGet:
		records, err := subscriber.Load(s.cfg.UserDBPath(), s.cfg.MaxUploadBytes)
		if err != nil {
			writeV1(w, r, CodeInternal, "subscriber database is invalid", nil)
			return
		}
		idx, ok := subscriber.Find(records, wanted)
		if !ok {
			writeV1(w, r, CodeNotFound, "subscriber not found", nil)
			return
		}
		writeV1(w, r, CodeOK, "ok", s.redactSubscribers(records[idx : idx+1])[0])
	case http.MethodPatch:
		s.handleV1SubscriberPatch(w, r, wanted)
	case http.MethodDelete:
		s.handleV1SubscriberDelete(w, r, wanted)
	default:
		writeV1(w, r, CodeMethod, "method not allowed, want GET, PATCH, or DELETE", nil)
	}
}

func (s *Server) handleV1SubscriberList(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePagination(r)
	if !ok {
		writeValidation(w, r, "pagination", "limit must be 1-200 and offset must be non-negative integers")
		return
	}
	records, err := subscriber.Load(s.cfg.UserDBPath(), s.cfg.MaxUploadBytes)
	if err != nil {
		writeV1(w, r, CodeInternal, "subscriber database is invalid", nil)
		return
	}
	total := len(records)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	items := s.redactSubscribers(records[offset:end])
	writeV1(w, r, CodeOK, "ok", map[string]any{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
}

func (s *Server) handleV1SubscriberCreate(w http.ResponseWriter, r *http.Request) {
	var req createSubscriberRequest
	if err := decodeJSONObject(w, r, &req, subscriberJSONLimit, true); err != nil {
		writeJSONDecodeError(w, r, err, subscriberJSONLimit)
		return
	}
	record, field, reason := recordFromCreate(req)
	if field != "" {
		writeValidation(w, r, field, reason)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := s.mgr.RunWhileStopped(ctx, func() error {
		subscriber.Mutex.Lock()
		defer subscriber.Mutex.Unlock()
		return subscriber.Create(s.cfg.UserDBPath(), s.cfg.MaxUploadBytes, record)
	})
	if errors.Is(err, lte.ErrCellMustBeStopped) {
		writeV1(w, r, CodeConflict, "cell must be stopped before changing subscribers", nil)
		return
	}
	if errors.Is(err, subscriber.ErrDuplicateIMSI) || errors.Is(err, subscriber.ErrDuplicateName) {
		writeV1(w, r, CodeConflict, err.Error(), nil)
		return
	}
	if err != nil {
		writeV1(w, r, CodeInternal, "subscriber create failed", nil)
		return
	}
	writeV1Status(w, r, http.StatusCreated, CodeOK, "subscriber created", record.Redacted())
}

func (s *Server) handleV1SubscriberPatch(w http.ResponseWriter, r *http.Request, wanted string) {
	var req patchSubscriberRequest
	if err := decodeJSONObject(w, r, &req, subscriberJSONLimit, true); err != nil {
		writeJSONDecodeError(w, r, err, subscriberJSONLimit)
		return
	}
	for field, present := range map[string]bool{
		"auth": len(req.Auth) != 0, "key": len(req.Key) != 0, "op_type": len(req.OPType) != 0,
		"op": len(req.OP) != 0, "opc": len(req.OPc) != 0, "amf": len(req.AMF) != 0, "sqn": len(req.SQN) != 0,
	} {
		if present {
			writeValidation(w, r, field, "authentication fields and sqn cannot be patched; use the stopped-cell replacement endpoint")
			return
		}
	}
	if req.Name == nil && req.QCI == nil && req.IPAlloc == nil {
		writeValidation(w, r, "body", "at least one of name, qci, or ip_alloc is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var updated subscriber.Record
	err := s.mgr.RunWhileStopped(ctx, func() error {
		subscriber.Mutex.Lock()
		defer subscriber.Mutex.Unlock()
		var err error
		updated, err = subscriber.UpdateMetadata(s.cfg.UserDBPath(), s.cfg.MaxUploadBytes, wanted, req.Name, req.QCI, req.IPAlloc)
		return err
	})
	if errors.Is(err, lte.ErrCellMustBeStopped) {
		writeV1(w, r, CodeConflict, "cell must be stopped before changing subscribers", nil)
		return
	}
	if errors.Is(err, subscriber.ErrNotFound) {
		writeV1(w, r, CodeNotFound, "subscriber not found", nil)
		return
	}
	if errors.Is(err, subscriber.ErrDuplicateName) {
		writeV1(w, r, CodeConflict, "subscriber name already exists", nil)
		return
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid ") {
			parts := strings.SplitN(strings.TrimPrefix(err.Error(), "invalid "), ": ", 2)
			if len(parts) == 2 {
				writeValidation(w, r, parts[0], parts[1])
				return
			}
		}
		writeV1(w, r, CodeInternal, "subscriber update failed", nil)
		return
	}
	writeV1(w, r, CodeOK, "subscriber updated", updated.Redacted())
}

func (s *Server) handleV1SubscriberDelete(w http.ResponseWriter, r *http.Request, wanted string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := s.mgr.RunWhileStopped(ctx, func() error {
		subscriber.Mutex.Lock()
		defer subscriber.Mutex.Unlock()
		return subscriber.Delete(s.cfg.UserDBPath(), s.cfg.MaxUploadBytes, wanted)
	})
	if errors.Is(err, lte.ErrCellMustBeStopped) {
		writeV1(w, r, CodeConflict, "cell must be stopped before changing subscribers", nil)
		return
	}
	if errors.Is(err, subscriber.ErrNotFound) {
		writeV1(w, r, CodeNotFound, "subscriber not found", nil)
		return
	}
	if err != nil {
		writeV1(w, r, CodeInternal, "subscriber delete failed", nil)
		return
	}
	writeV1(w, r, CodeOK, "subscriber deleted", map[string]any{
		"imsi": wanted, "disconnected": false,
	})
}

func (s *Server) handleV1SubscriberUpload(w http.ResponseWriter, r *http.Request) {
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
	f, _, err := r.FormFile("userdb")
	if err != nil || f == nil {
		writeValidation(w, r, "userdb", "required")
		return
	}
	defer f.Close()
	payload, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxUploadBytes+1))
	if err != nil {
		writeV1(w, r, CodeInternal, "upload failed", nil)
		return
	}
	if len(payload) == 0 {
		writeValidation(w, r, "userdb", "empty")
		return
	}
	if int64(len(payload)) > s.cfg.MaxUploadBytes {
		writeV1(w, r, CodeTooLarge, "upload exceeds limit", map[string]any{"max_bytes": s.cfg.MaxUploadBytes})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	count := 0
	err = s.mgr.RunWhileStopped(ctx, func() error {
		subscriber.Mutex.Lock()
		defer subscriber.Mutex.Unlock()
		var err error
		count, err = subscriber.ReplacePreservingSQN(s.cfg.UserDBPath(), s.cfg.MaxUploadBytes, payload)
		return err
	})
	if errors.Is(err, lte.ErrCellMustBeStopped) {
		writeV1(w, r, CodeConflict, "cell must be stopped before replacing subscribers", nil)
		return
	}
	if errors.Is(err, subscriber.ErrIncomingInvalid) {
		writeV1(w, r, CodeInvalid, "subscriber database validation failed", nil)
		return
	}
	if err != nil {
		writeV1(w, r, CodeInternal, "subscriber database replacement failed", nil)
		return
	}
	writeV1(w, r, CodeOK, "subscriber database replaced", map[string]any{
		"bytes": len(payload), "subscribers": count, "sqn_policy": "preserved_for_existing_imsi",
	})
}

func recordFromCreate(req createSubscriberRequest) (subscriber.Record, string, string) {
	value, opType := "", strings.ToLower(strings.TrimSpace(req.OPType))
	op, opc := strings.TrimSpace(req.OP), strings.TrimSpace(req.OPc)
	if (op == "") == (opc == "") {
		return subscriber.Record{}, "op", "exactly one of op or opc is required"
	}
	if op != "" {
		value = op
		if opType == "" {
			opType = "op"
		}
		if opType != "op" {
			return subscriber.Record{}, "op_type", "must match the supplied op field"
		}
	} else {
		value = opc
		if opType == "" {
			opType = "opc"
		}
		if opType != "opc" {
			return subscriber.Record{}, "op_type", "must match the supplied opc field"
		}
	}
	r := subscriber.Record{
		Name: strings.TrimSpace(req.Name), Auth: strings.ToLower(strings.TrimSpace(req.Auth)),
		IMSI: strings.TrimSpace(req.IMSI), Key: strings.ToLower(strings.TrimSpace(req.Key)),
		OPType: opType, OPValue: strings.ToLower(value), AMF: strings.ToLower(strings.TrimSpace(req.AMF)),
		SQN: strings.ToLower(strings.TrimSpace(req.SQN)), QCI: req.QCI,
		IPAlloc: strings.ToLower(strings.TrimSpace(req.IPAlloc)),
	}
	field, reason := subscriber.ValidateNew(r)
	return r, field, reason
}

func parsePagination(r *http.Request) (limit, offset int, ok bool) {
	limit, offset = 50, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			return 0, 0, false
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, false
		}
	}
	return limit, offset, true
}

func (s *Server) redactSubscribers(records []subscriber.Record) []subscriber.Public {
	items := make([]subscriber.Public, 0, len(records))
	for _, record := range records {
		items = append(items, record.Redacted())
	}
	result := s.currentUESnapshot()
	if result.State != "current" || result.Snapshot == nil {
		return items
	}
	registered := make(map[string]bool, len(result.Snapshot.Sessions))
	for _, session := range result.Snapshot.Sessions {
		if session.IMSI != "" && session.EMMState == "registered" {
			registered[session.IMSI] = true
		}
	}
	for i := range items {
		active := registered[items[i].IMSI]
		items[i].Active = &active
	}
	return items
}

func validIMSI(value string) bool {
	if len(value) != 15 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func writeValidation(w http.ResponseWriter, r *http.Request, field, reason string) {
	writeV1(w, r, CodeInvalid, "validation failed", map[string]any{
		"errors": []FieldError{{Field: field, Reason: reason}},
	})
}

func writeJSONDecodeError(w http.ResponseWriter, r *http.Request, err error, maxBytes int64) {
	if errors.Is(err, errJSONTooLarge) {
		writeV1(w, r, CodeTooLarge, "JSON body exceeds limit", map[string]any{"max_bytes": maxBytes})
		return
	}
	writeV1(w, r, CodeMalformed, "malformed JSON body", nil)
}
