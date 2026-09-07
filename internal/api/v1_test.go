package api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %q", rec.Body.String())
	}
	if _, ok := m["request_id"]; !ok {
		t.Fatalf("missing request_id: %v", m)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID header")
	}
	return m
}

func TestV1_NotFound(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	if m := decodeEnvelope(t, rec); m["code"] != float64(40401) {
		t.Fatalf("want 40401: %v", m)
	}
}

func TestV1_UnknownRootPathUsesStandardEnvelope(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 || decodeEnvelope(t, rec)["code"] != float64(CodeNotFound) {
		t.Fatalf("unknown path must use standard 404: %d %q", rec.Code, rec.Body.String())
	}
}

func TestV1_MethodNotAllowed(t *testing.T) {
	s, _ := testServer(t)
	// Every documented path with a disallowed method must answer 405
	// (proves the route table and dispatcher agree).
	valid := map[string]map[string]bool{}
	for _, rt := range v1Routes {
		if valid[rt.path] == nil {
			valid[rt.path] = map[string]bool{}
		}
		valid[rt.path][rt.method] = true
	}
	probe := map[string]string{ // one path per group is enough
		"/api/v1/cell": "PATCH", "/api/v1/network": "POST", "/api/v1/ues": "POST",
		"/api/v1/diagnostics/connectivity": "POST",
		"/api/v1/crack/jobs":               "GET", "/api/v1/crack/result": "DELETE",
		"/api/v1/subscribers": "PUT", "/api/v1/config/subscribers": "GET", "/api/v1/config/wordlist": "DELETE",
		"/api/v1/captures/lte-data": "POST", "/api/v1/simcards": "GET",
		"/api/v1/profile": "POST", "/api/v1/health": "POST",
	}
	for path, method := range probe {
		if valid[path][method] {
			t.Fatalf("probe method %s is actually valid for %s", method, path)
		}
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 405 || decodeEnvelope(t, rec)["code"] != float64(40501) {
			t.Fatalf("%s %s: want 405 got %d (%s)", method, path, rec.Code, rec.Body.String())
		}
	}
}

func TestV1_ConnectivityDiagnosticsIsReadOnlyAndCredentialFree(t *testing.T) {
	s, cfg := testServer(t)
	logBody := "Attach request -- eNB-UE S1AP Id: 1\n" +
		"Attach Request -- IMSI: 001019876543210\n" +
		"UE Authentication Accepted.\n" +
		"Security Mode Command Complete -- IMSI: 001019876543210\n" +
		"SPGW: get_new_ue_ipv4 pool ip addr 172.16.0.2\n" +
		"UL NAS: Received Attach Complete\n" +
		"Activated EPS Bearer: Bearer id 5\n"
	if err := os.WriteFile(cfg.LogPath(cfg.EPCLogName), []byte(logBody), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/connectivity", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics must return partial evidence as 200: %d %s", rec.Code, rec.Body.String())
	}
	m := decodeEnvelope(t, rec)
	data, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing diagnostic data: %v", m)
	}
	registration, _ := data["registration"].(map[string]any)
	if registration["state"] != "attach_complete_observed" || registration["scope"] != "aggregate_only" {
		t.Fatalf("unexpected registration evidence: %v", registration)
	}
	pdn, _ := data["pdn"].(map[string]any)
	if pdn["state"] != "bearer_activation_observed" {
		t.Fatalf("unexpected PDN evidence: %v", pdn)
	}
	body := rec.Body.String()
	for _, secret := range []string{"001019876543210", `"username"`, `"hash"`, `"password"`} {
		if strings.Contains(body, secret) {
			t.Fatalf("diagnostics exposed credential/subscriber data %q: %s", secret, body)
		}
	}
	if s.mgr.IsRunning().Running {
		t.Fatal("read-only diagnostics changed cell state")
	}
}

func TestV1_CrackDistinguishesMissingCapture(t *testing.T) {
	s, cfg := testServer(t)
	if err := os.WriteFile(cfg.LogPath(cfg.EPCLogName), []byte("ESM Info: APN test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/crack/jobs", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	data, _ := m["data"].(map[string]any)
	if rec.Code != http.StatusPreconditionFailed || m["code"] != float64(CodePrecondition) ||
		data["reason"] != "capture_missing" {
		t.Fatalf("missing capture must not be reported as no handshake: %d %v", rec.Code, m)
	}
}

// Synthetic framing only: one byte of opaque payload, no authentication data.
func diagnosticPCAPFixture() []byte {
	b := make([]byte, 41)
	copy(b, []byte{0xd4, 0xc3, 0xb2, 0xa1})
	binary.LittleEndian.PutUint16(b[4:6], 2)
	binary.LittleEndian.PutUint16(b[6:8], 4)
	binary.LittleEndian.PutUint32(b[16:20], 65535)
	binary.LittleEndian.PutUint32(b[20:24], 150)
	binary.LittleEndian.PutUint32(b[32:36], 1)
	binary.LittleEndian.PutUint32(b[36:40], 1)
	return b
}

func TestV1_CrackDistinguishesMissingTsharkAndUndecodableCapture(t *testing.T) {
	for _, tc := range []struct {
		name       string
		bin        func(configDir string) string
		wantHTTP   int
		wantCode   int
		wantReason string
	}{
		{
			name:     "missing tshark",
			bin:      func(dir string) string { return filepath.Join(dir, "missing-tshark") },
			wantHTTP: http.StatusServiceUnavailable, wantCode: CodeDependency, wantReason: "tshark_missing",
		},
		{
			name: "undecodable capture",
			// The Go test binary rejects tshark flags and exits non-zero. This is
			// a portable subprocess fixture for the decoder-failure path.
			bin:      func(string) string { return os.Args[0] },
			wantHTTP: http.StatusUnprocessableEntity, wantCode: CodeUnprocessable, wantReason: "capture_decode_failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, cfg := testServer(t)
			s.cfg.TsharkBin = tc.bin(cfg.DataDir)
			if err := os.WriteFile(cfg.LogPath(cfg.EPCLogName), []byte("ESM Info: APN test\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfg.LogPath(cfg.PcapS1AP), diagnosticPCAPFixture(), 0o600); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/crack/jobs", nil)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			m := decodeEnvelope(t, rec)
			data, _ := m["data"].(map[string]any)
			if rec.Code != tc.wantHTTP || m["code"] != float64(tc.wantCode) || data["reason"] != tc.wantReason {
				t.Fatalf("unexpected classification: %d %v", rec.Code, m)
			}
		})
	}
}

func TestV1_ConnectivityCaptureIntegrityMetadata(t *testing.T) {
	wrongLink := diagnosticPCAPFixture()
	binary.LittleEndian.PutUint32(wrongLink[20:24], 1)
	for _, tc := range []struct {
		name, reason, state string
		capture             []byte
	}{
		{"invalid format", "capture_format_invalid", "unavailable", []byte("not-a-pcap")},
		{"partial record", "capture_incomplete", "unknown", diagnosticPCAPFixture()[:40]},
		{"header only", "capture_no_packets", "not_collected", diagnosticPCAPFixture()[:24]},
		{"wrong link", "capture_linktype_unsupported", "unavailable", wrongLink},
		{"missing decoder", "tshark_missing", "unavailable", diagnosticPCAPFixture()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, cfg := testServer(t)
			s.cfg.TsharkBin = filepath.Join(cfg.DataDir, "missing-tshark")
			if err := os.WriteFile(cfg.LogPath(cfg.PcapS1AP), tc.capture, 0o600); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/connectivity", nil))
			m := decodeEnvelope(t, rec)
			if rec.Code != 200 || m["code"] != float64(0) {
				t.Fatalf("%d: %v", rec.Code, m)
			}
			data := m["data"].(map[string]any)
			chap := data["chap"].(map[string]any)
			if chap["reason"] != tc.reason || chap["state"] != tc.state || chap["scan_complete"] != false {
				t.Fatalf("wrong integrity classification: %v", chap)
			}
			for _, key := range []string{`"username"`, `"password"`, `"challenge"`, `"response"`, `"hash"`} {
				if strings.Contains(rec.Body.String(), key) {
					t.Fatalf("sensitive field %s", key)
				}
			}
			left, err := filepath.Glob(cfg.LogPath(".lte-s1ap-prefix-*.pcap"))
			if err != nil || len(left) != 0 {
				t.Fatalf("temporary capture leak: %v %v", left, err)
			}
			if s.mgr.IsRunning().Running {
				t.Fatal("diagnostic changed cell state")
			}
		})
	}
}

func TestV1_ConnectivityDiagnosticsHonorsAuth(t *testing.T) {
	t.Setenv("LTE_API_TOKEN", "diagnostic-secret")
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/connectivity", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || decodeEnvelope(t, rec)["code"] != float64(CodeUnauthorized) {
		t.Fatalf("unauthorized diagnostic request passed: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/connectivity", nil)
	req.Header.Set("Authorization", "Bearer diagnostic-secret")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || decodeEnvelope(t, rec)["code"] != float64(CodeOK) {
		t.Fatalf("authorized diagnostic request failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestV1_ConnectivityDiagnosticsRejectsConcurrentRun(t *testing.T) {
	s, _ := testServer(t)
	// Deterministically represent one in-flight diagnostic without starting
	// subprocesses; the non-blocking gate is the concurrency contract.
	s.diagGate <- struct{}{}
	defer func() { <-s.diagGate }()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/connectivity", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	data, _ := m["data"].(map[string]any)
	if rec.Code != http.StatusTooManyRequests || m["code"] != float64(CodeTooManyRequests) ||
		data["reason"] != "diagnostic_busy" {
		t.Fatalf("concurrent diagnostic was not rejected: %d %v", rec.Code, m)
	}
}

func TestV1_StartValidation(t *testing.T) {
	s, _ := testServer(t)
	// Malformed.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cell", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 || decodeEnvelope(t, rec)["code"] != float64(40001) {
		t.Fatalf("malformed: %d %s", rec.Code, rec.Body.String())
	}
	// Unknown band must NOT silently fall back (legacy did).
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cell",
		bytes.NewBufferString(`{"band":"400","apn":"a","mcc":"001","mnc":"01","network":"eth0"}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	if rec.Code != 422 || m["code"] != float64(42201) {
		t.Fatalf("bad band: %d %v", rec.Code, m)
	}
	// Bad gain + bad n_prb pin the whitelist.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cell",
		bytes.NewBufferString(`{"band":"7","apn":"a","mcc":"001","mnc":"01","network":"eth0","tx_gain":999,"n_prb":99}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m = decodeEnvelope(t, rec)
	data, _ := m["data"].(map[string]any)
	errs, _ := data["errors"].([]any)
	if rec.Code != 422 || len(errs) != 2 {
		t.Fatalf("want 2 field errors: %d %v", rec.Code, m)
	}
}

func TestV1_StopIdempotent(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cell", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	if rec.Code != 200 || m["code"] != float64(0) {
		t.Fatalf("idle stop should succeed: %d %v", rec.Code, m)
	}
}

func TestV1_NetworkIsReadOnlyConfigurationSnapshot(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	data, ok := m["data"].(map[string]any)
	if rec.Code != http.StatusOK || !ok || data["active"] != false {
		t.Fatalf("unexpected idle network plan: %d %v", rec.Code, m)
	}
	for _, field := range []string{"ue_subnet", "sgi_address", "ue_access"} {
		if _, exists := data[field]; !exists {
			t.Fatalf("network plan omitted %s: %v", field, data)
		}
	}
	for _, inferred := range []string{"internet_reachable", "dns_reachable", "connected"} {
		if _, exists := data[inferred]; exists {
			t.Fatalf("network configuration endpoint inferred %s: %v", inferred, data)
		}
	}
}

func TestV1_UEsReportsCellStoppedWithoutLogFallback(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ues", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	data := m["data"].(map[string]any)
	if rec.Code != http.StatusOK || data["state"] != "cell_stopped" || len(data["sessions"].([]any)) != 0 {
		t.Fatalf("unexpected stopped snapshot: %d %v", rec.Code, m)
	}
}

func TestV1_Captures(t *testing.T) {
	s, cfg := testServer(t)
	// Missing file -> 404.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/captures/lte-data", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	// Unknown id -> 404.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/captures/nope", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	// Present file -> download by standard string id.
	if err := os.WriteFile(cfg.LogPath(cfg.PcapLTEData), []byte("pcap"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lte-data"} {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/captures/"+id, nil)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 || rec.Body.String() != "pcap" {
			t.Fatalf("id %s: %d %q", id, rec.Code, rec.Body.String())
		}
	}
}

func TestV1_Uploads(t *testing.T) {
	s, cfg := testServer(t)
	// Missing field -> 422.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/subscribers", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 422 {
		t.Fatalf("want 422 got %d %s", rec.Code, rec.Body.String())
	}
	// Happy path carries the restart note.
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("userdb", "user_db.csv")
	_, _ = fw.Write([]byte("ue0,mil,001010123456789,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8000,000000001234,7,dynamic\n"))
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/config/subscribers", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	if rec.Code != 200 {
		t.Fatalf("upload: %d %v", rec.Code, m)
	}
	if m["data"].(map[string]any)["sqn_policy"] != "preserved_for_existing_imsi" {
		t.Fatalf("SQN policy missing: %v", m)
	}
	if _, err := os.Stat(cfg.UserDBPath()); err != nil {
		t.Fatal(err)
	}
}

func TestV1_SubscriberCRUDIsRedactedAndSQNSafe(t *testing.T) {
	s, cfg := testServer(t)
	secretKey := "00112233445566778899aabbccddeeff"
	secretOPC := "63bfa50ee6523365ff14c1f45f88737d"
	body := `{"name":"phone","auth":"mil","imsi":"001010123456789","key":"` + secretKey + `","opc":"` + secretOPC + `","op_type":"opc","amf":"8001","sqn":"000000001234","qci":7,"ip_alloc":"dynamic"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscribers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secretKey) || strings.Contains(rec.Body.String(), secretOPC) || strings.Contains(rec.Body.String(), "000000001234") {
		t.Fatalf("create response leaked authentication material: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/subscribers?limit=1&offset=0", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	data := m["data"].(map[string]any)
	items := data["items"].([]any)
	if rec.Code != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["authorized"] != true || items[0].(map[string]any)["active"] != nil {
		t.Fatalf("list does not distinguish authorization from activity: %d %v", rec.Code, m)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/subscribers/001010123456789", strings.NewReader(`{"sqn":"00000000ffff"}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("SQN patch accepted: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/subscribers/001010123456789", strings.NewReader(`{"key":null}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("null protected field bypassed presence check: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/subscribers/001010123456789", strings.NewReader(`{"name":"updated","qci":9}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metadata patch: %d %s", rec.Code, rec.Body.String())
	}
	disk, err := os.ReadFile(cfg.UserDBPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(disk), secretKey) || !strings.Contains(string(disk), "000000001234") {
		t.Fatalf("metadata patch changed protected columns: %s", disk)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/subscribers/001010123456789", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), secretKey) {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestV1_SubscriberValidationAndPagination(t *testing.T) {
	s, _ := testServer(t)
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/subscribers?limit=201", ""},
		{http.MethodGet, "/api/v1/subscribers/not-an-imsi", ""},
		{http.MethodPost, "/api/v1/subscribers", `{"name":"phone","auth":"mil","imsi":"001","key":"secret","opc":"secret","amf":"8001","sqn":"000000001234","qci":7,"ip_alloc":"dynamic"}`},
		{http.MethodPost, "/api/v1/subscribers", `{"name":"phone","auth":"mil","imsi":"001010123456789","key":"00112233445566778899aabbccddeeff","opc":"63bfa50ee6523365ff14c1f45f88737d","amf":"8001","sqn":"000000001234","qci":7,"ip_alloc":"172.16.0.2"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s %s: want 422 got %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "secret") {
			t.Fatalf("validation response leaked submitted value: %s", rec.Body.String())
		}
	}
}

func TestV1_SIMValidation(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/simcards", bytes.NewBufferString(`{"mcc":"001"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 422 {
		t.Fatalf("want 422 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestV1_HealthAndProfile(t *testing.T) {
	s, _ := testServer(t)
	for _, p := range []string{"/api/v1/health", "/api/v1/profile", "/api/v1/cell"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", p, rec.Code, rec.Body.String())
		}
		decodeEnvelope(t, rec)
	}
}

func TestV1_Auth(t *testing.T) {
	t.Setenv("LTE_API_TOKEN", "secret")
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200 got %d", rec.Code)
	}
	// Unknown paths are covered by the same gate, so unauthenticated probes do
	// not reveal whether a resource exists.
	req = httptest.NewRequest(http.MethodPost, "/stop", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("unknown route must also require token: %d", rec.Code)
	}
}

func createOwnedSubscriber(t *testing.T, s *Server, imsi string) {
	t.Helper()
	body := `{"name":"ownerphone","auth":"mil","imsi":"` + imsi + `","key":"00112233445566778899aabbccddeeff","opc":"63bfa50ee6523365ff14c1f45f88737d","op_type":"opc","amf":"8001","sqn":"000000001234","qci":7,"ip_alloc":"dynamic"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscribers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed subscriber: %d %s", rec.Code, rec.Body.String())
	}
}

func TestV1_CrackJobsConsentValidation(t *testing.T) {
	s, cfg := testServer(t)
	owned := "001010123456789"
	createOwnedSubscriber(t, s, owned)
	if err := os.WriteFile(cfg.LogPath(cfg.EPCLogName), []byte("ESM Info: APN srsapn\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Malformed JSON -> 400, no credential handling.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/crack/jobs", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed consent: %d %s", rec.Code, rec.Body.String())
	}

	// Bad IMSI shape -> 422 with field detail.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/crack/jobs",
		strings.NewReader(`{"imsi":"001","confirm_ownership":true}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "imsi") {
		t.Fatalf("bad imsi: %d %s", rec.Code, rec.Body.String())
	}

	// Missing confirm_ownership -> 422 (no silent third-party audit).
	req = httptest.NewRequest(http.MethodPost, "/api/v1/crack/jobs",
		strings.NewReader(`{"imsi":"`+owned+`"}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "confirm_ownership") {
		t.Fatalf("missing consent: %d %s", rec.Code, rec.Body.String())
	}

	// Unknown IMSI (not provisioned here) -> 404, not a handshake verdict.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/crack/jobs",
		strings.NewReader(`{"imsi":"001010999999999","confirm_ownership":true}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown owned imsi: %d %s", rec.Code, rec.Body.String())
	}

	// Subscriber membership cannot bind the capture: return 412 before extraction.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/crack/jobs",
		strings.NewReader(`{"imsi":"`+owned+`","confirm_ownership":true}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	if rec.Code != http.StatusPreconditionFailed || m["message"] != "target binding unavailable" {
		t.Fatalf("owned target must fail binding: %d %v", rec.Code, m)
	}
	if strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("failure must not carry a password: %s", rec.Body.String())
	}
}

func TestV1_CrackResultIMSIQuery(t *testing.T) {
	s, cfg := testServer(t)
	owned := "001010123456780"
	createOwnedSubscriber(t, s, owned)
	if err := os.WriteFile(cfg.LogPath(cfg.EPCLogName), []byte("ESM Info: APN srsapn\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crack/result?imsi=001010999999999", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown query imsi: %d %s", rec.Code, rec.Body.String())
	}

	// Owned query with no capture -> 412, still credential-free.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/crack/result?imsi="+owned, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionFailed || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("owned empty result: %d %s", rec.Code, rec.Body.String())
	}
}

func TestV1AuditTargetDatabaseFailure(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		for _, broken := range []string{"malformed", "directory"} {
			t.Run(method+"/"+broken, func(t *testing.T) {
				s, cfg := testServer(t)
				if broken == "directory" {
					if err := os.Mkdir(cfg.UserDBPath(), 0o700); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(cfg.UserDBPath(), []byte("invalid,row\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				url, body := "/api/v1/crack/jobs", `{"imsi":"001010123456789","confirm_ownership":true}`
				if method == http.MethodGet {
					url, body = "/api/v1/crack/result?imsi=001010123456789", ""
				}
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
				m := decodeEnvelope(t, rec)
				if rec.Code != http.StatusInternalServerError || m["code"] != float64(CodeInternal) || m["message"] != "subscriber database unavailable" {
					t.Fatalf("DB failure must be an independent server error: %d %v", rec.Code, m)
				}
				for _, key := range []string{`"username"`, `"password"`, `"hash"`} {
					if strings.Contains(rec.Body.String(), key) {
						t.Fatalf("DB error exposes %s", key)
					}
				}
			})
		}
	}
}

func TestV1AuditResultValidatesTargetShape(t *testing.T) {
	s, _ := testServer(t)
	for _, query := range []string{"imsi=001", "imsi=", "imsi=%20001010123456789", "imsi=001010123456789&imsi=001010123456788"} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/crack/result?"+query, nil))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid target %s: %d %s", query, rec.Code, rec.Body.String())
		}
	}
}

func TestV1AuditTargetBlocksAllExternalWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable traps")
	}
	s, cfg := testServer(t)
	const imsi = "001010123456789"
	createOwnedSubscriber(t, s, imsi)
	bin := t.TempDir()
	marker := filepath.Join(bin, "called")
	t.Setenv("AUDIT_TEST_MARKER", marker)
	trap := "#!/bin/sh\nprintf 'called' >> \"$AUDIT_TEST_MARKER\"\nexit 1\n"
	for _, name := range []string{"ps", "tshark", "hashcat", "kill"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(trap), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	s.cfg.TsharkBin = filepath.Join(bin, "tshark")
	s.cfg.HashcatBin = filepath.Join(bin, "hashcat")
	s.mgr = nil // Any attempted cell stop is a test failure via the panic/500 path.
	capture := diagnosticPCAPFixture()
	if err := os.WriteFile(cfg.LogPath(cfg.PcapS1AP), capture, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.WordlistPath(), []byte("synthetic-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		url, body := "/api/v1/crack/jobs", `{"imsi":"`+imsi+`","confirm_ownership":true}`
		if method == http.MethodGet {
			url, body = "/api/v1/crack/result?imsi="+imsi, ""
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
		m := decodeEnvelope(t, rec)
		data, _ := m["data"].(map[string]any)
		if rec.Code != http.StatusPreconditionFailed || m["message"] != "target binding unavailable" || data["reason"] != "target_binding_unavailable" || data["ownership_verified"] != false {
			t.Fatalf("target must fail closed before all external work: %d %v", rec.Code, m)
		}
		for _, key := range []string{`"username"`, `"password"`, `"hash"`} {
			if strings.Contains(rec.Body.String(), key) {
				t.Fatalf("target exposes %s", key)
			}
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("external tool invoked: %v", err)
	}
	got, err := os.ReadFile(cfg.LogPath(cfg.PcapS1AP))
	if err != nil || !bytes.Equal(got, capture) {
		t.Fatalf("capture changed: %v", err)
	}

	// Legacy result still runs the aggregate flow, with ownership always false.
	ps := "#!/bin/sh\nprintf '123 S hashcat\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte(ps), 0o700); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/crack/result", nil))
	m := decodeEnvelope(t, rec)
	data, _ := m["data"].(map[string]any)
	if rec.Code != http.StatusOK || data["state"] != "running" || data["ownership_verified"] != false {
		t.Fatalf("legacy flow changed: %d %v", rec.Code, m)
	}
}
