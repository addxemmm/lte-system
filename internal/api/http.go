// Package api implements the HTTP layer: legacy routes (frozen) + /api/v1.
//
// v1 contract (see docs/API.md and docs/api/openapi.yaml):
//   - Proper HTTP status codes (200/201/400/401/404/405/409/412/413/422/500/503)
//   - JSON envelope {"code","message","data","request_id"}; code 0 = success
//   - X-Request-ID response header; per-request audit log line
//   - Optional bearer auth via LTE_API_TOKEN (when set, all routes require it)
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Business codes for the v1 envelope. HTTP status is derived from code/1000.
const (
	CodeOK           = 0
	CodeMalformed    = 40001 // body not JSON / wrong shape (HTTP 400)
	CodeUnauthorized = 40101 // missing or bad bearer token (HTTP 401)
	CodeNotFound     = 40401 // unknown resource id (HTTP 404)
	CodeMethod       = 40501 // method not allowed (HTTP 405)
	CodeConflict     = 40901 // cell/crack already running, card exists (HTTP 409)
	CodePrecondition = 41201 // cell not running, no UE/CHAP/data, no card (HTTP 412)
	CodeTooLarge     = 41301 // JSON body or upload exceeds limit (HTTP 413)
	CodeInvalid      = 42201 // validation failed, see data.errors (HTTP 422)
	CodeInternal     = 50001 // unexpected failure (HTTP 500)
	CodeNoHardware   = 50301 // no SDR / reader attached (HTTP 503)
)

// FieldError describes one rejected field for 422 responses.
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// Envelope is the v1 response body.
type Envelope struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id"`
}

type ctxKey int

const requestIDKey ctxKey = iota

var errJSONTooLarge = errors.New("JSON body exceeds limit")

// RequestID returns the request id attached by middleware ("" when absent).
func RequestID(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000")
	}
	return hex.EncodeToString(b[:])
}

// decodeJSONObject accepts exactly one JSON object. Decoding into a RawMessage
// first is deliberate: encoding/json otherwise accepts null for struct targets,
// and a single Decode silently ignores a second value or trailing garbage.
func decodeJSONObject(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, disallowUnknown bool) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return classifyJSONError(err)
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err == nil {
		return errors.New("JSON body must contain exactly one value")
	} else if err != io.EOF {
		return classifyJSONError(err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("JSON body must be an object")
	}
	obj := json.NewDecoder(bytes.NewReader(raw))
	if disallowUnknown {
		obj.DisallowUnknownFields()
	}
	return obj.Decode(dst)
}

func classifyJSONError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return errJSONTooLarge
	}
	return err
}

// writeV1 writes a v1 envelope with the HTTP status derived from code.
// Codes are HTTP*100+seq (e.g. 40401 -> 404); code 0 -> 200.
func writeV1(w http.ResponseWriter, r *http.Request, code int, message string, data any) {
	status := http.StatusOK
	if code != 0 {
		status = code / 100
	}
	if status < 100 || status > 599 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", RequestID(r))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Code: code, Message: message, Data: data, RequestID: RequestID(r),
	})
}

// writeV1Status writes a v1 envelope with an explicit HTTP status
// (for 201/202 cases where status is not derivable from code).
func writeV1Status(w http.ResponseWriter, r *http.Request, status, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", RequestID(r))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Code: code, Message: message, Data: data, RequestID: RequestID(r),
	})
}

// statusRecorder captures the status code for the audit log.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wroteHeader {
		return
	}
	s.status = code
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if !s.wroteHeader {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(p)
}

// chain applies middlewares: recover -> request id + audit log -> auth.
func chain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		rec := &statusRecorder{ResponseWriter: w}
		rec.Header().Set("X-Request-ID", id)
		start := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("rid=%s panic recovered: %v", id, recovered)
				// Once a handler has committed a response, appending a JSON error
				// would corrupt it. Audit the committed status instead.
				if !rec.wroteHeader {
					writeV1(rec, r, CodeInternal, "internal error", nil)
				}
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			log.Printf("rid=%s %s %s -> %d (%s)", id, r.Method, r.URL.Path, status, time.Since(start).Round(time.Millisecond))
		}()
		if !authorized(r) {
			rec.Header().Set("Content-Type", "application/json")
			rec.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(rec).Encode(Envelope{
				Code: CodeUnauthorized, Message: "unauthorized: bad or missing bearer token", RequestID: id,
			})
			return
		}
		next.ServeHTTP(rec, r)
	})
}

// authorized checks the optional bearer token. Empty LTE_API_TOKEN = open
// LAN mode (a warning is logged once at startup by main).
func authorized(r *http.Request) bool {
	want := strings.TrimSpace(os.Getenv("LTE_API_TOKEN"))
	if want == "" {
		return true
	}
	got := r.Header.Get("Authorization")
	if !strings.HasPrefix(got, "Bearer ") {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(got, "Bearer ")) == want
}

// notFoundV1 renders unknown paths as a v1 404 envelope (keeps the
// contract machine-readable instead of Go's default plain-text 404).
func notFoundV1() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeV1(w, r, CodeNotFound, "not found: "+r.URL.Path, nil)
	})
}

// methodOnly wraps a handler, enforcing one HTTP method with a 405 envelope.
func methodOnly(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeV1(w, r, CodeMethod, "method not allowed, want "+method, nil)
			return
		}
		h(w, r)
	}
}
