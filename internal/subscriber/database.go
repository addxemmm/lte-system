package subscriber

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const MaxRecords = 4096

var (
	hex32 = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	hex4  = regexp.MustCompile(`^[0-9a-fA-F]{4}$`)
	hex12 = regexp.MustCompile(`^[0-9a-fA-F]{12}$`)
	imsi  = regexp.MustCompile(`^[0-9]{15}$`)
	name  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

var (
	ErrDuplicateIMSI   = errors.New("subscriber IMSI already exists")
	ErrDuplicateName   = errors.New("subscriber name already exists")
	ErrNotFound        = errors.New("subscriber not found")
	ErrIncomingInvalid = errors.New("incoming subscriber database is invalid")
)

// Record is one validated srsRAN user_db.csv row. Authentication material is
// deliberately excluded from JSON so it cannot be exposed accidentally.
type Record struct {
	Name    string `json:"name"`
	Auth    string `json:"auth"`
	IMSI    string `json:"imsi"`
	Key     string `json:"-"`
	OPType  string `json:"op_type"`
	OPValue string `json:"-"`
	AMF     string `json:"-"`
	SQN     string `json:"-"`
	QCI     int    `json:"qci"`
	IPAlloc string `json:"ip_alloc"`
}

// Public is the redacted API representation of a configured subscriber.
// Active remains nil until it can be joined to an authoritative runtime
// telemetry snapshot; configuration alone never implies attachment.
type Public struct {
	Name             string `json:"name"`
	Auth             string `json:"auth"`
	IMSI             string `json:"imsi"`
	OPType           string `json:"op_type"`
	QCI              int    `json:"qci"`
	IPAlloc          string `json:"ip_alloc"`
	Authorized       bool   `json:"authorized"`
	Active           *bool  `json:"active"`
	CredentialStatus string `json:"credential_status"`
}

func (r Record) Redacted() Public {
	return Public{
		Name: r.Name, Auth: r.Auth, IMSI: r.IMSI, OPType: r.OPType,
		QCI: r.QCI, IPAlloc: r.IPAlloc, Authorized: true,
		CredentialStatus: "configured",
	}
}

// Load reads and validates a bounded subscriber database. A missing file is an
// empty database; malformed rows fail the whole snapshot rather than vanishing.
func Load(path string, maxBytes int64) ([]Record, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f, maxBytes)
}

func Parse(src io.Reader, maxBytes int64) ([]Record, error) {
	if maxBytes <= 0 {
		return nil, errors.New("subscriber database size limit is invalid")
	}
	payload, err := io.ReadAll(io.LimitReader(src, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read subscriber database: %w", err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, errors.New("subscriber database exceeds size limit")
	}
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 64*1024), int(min64(maxBytes, 1<<20)))
	records := make([]Record, 0)
	seenIMSI := make(map[string]struct{})
	seenName := make(map[string]struct{})
	seenIP := make(map[string]struct{})
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		reader := csv.NewReader(strings.NewReader(line))
		reader.FieldsPerRecord = 10
		reader.TrimLeadingSpace = true
		cols, err := reader.Read()
		if err != nil {
			return nil, fmt.Errorf("subscriber row %d must contain exactly 10 columns", lineNo)
		}
		for i := range cols {
			cols[i] = strings.TrimSpace(cols[i])
		}
		qci, err := strconv.Atoi(cols[8])
		if err != nil {
			return nil, fmt.Errorf("subscriber row %d has invalid qci", lineNo)
		}
		r := Record{
			Name: cols[0], Auth: strings.ToLower(cols[1]), IMSI: cols[2],
			Key: strings.ToLower(cols[3]), OPType: strings.ToLower(cols[4]),
			OPValue: strings.ToLower(cols[5]), AMF: strings.ToLower(cols[6]),
			SQN: strings.ToLower(cols[7]), QCI: qci, IPAlloc: strings.ToLower(cols[9]),
		}
		if field, reason := validateRecord(r, true); field != "" {
			return nil, fmt.Errorf("subscriber row %d has invalid %s: %s", lineNo, field, reason)
		}
		if _, ok := seenIMSI[r.IMSI]; ok {
			return nil, fmt.Errorf("subscriber row %d duplicates imsi", lineNo)
		}
		seenIMSI[r.IMSI] = struct{}{}
		if _, ok := seenName[r.Name]; ok {
			return nil, fmt.Errorf("subscriber row %d duplicates name", lineNo)
		}
		seenName[r.Name] = struct{}{}
		if r.IPAlloc != "dynamic" {
			if _, ok := seenIP[r.IPAlloc]; ok {
				return nil, fmt.Errorf("subscriber row %d duplicates static ip_alloc", lineNo)
			}
			seenIP[r.IPAlloc] = struct{}{}
		}
		records = append(records, r)
		if len(records) > MaxRecords {
			return nil, errors.New("subscriber database exceeds record limit")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read subscriber database: %w", err)
	}
	return records, nil
}

// ValidateNew validates one API-created subscriber. Static allocation is
// intentionally rejected by this API; existing static values remain readable.
func ValidateNew(r Record) (field, reason string) {
	return validateRecord(r, false)
}

func validateRecord(r Record, allowExistingStatic bool) (field, reason string) {
	switch {
	case !name.MatchString(r.Name):
		return "name", "must be 1-64 letters, digits, dot, underscore, or hyphen"
	case r.Auth != "mil" && r.Auth != "xor":
		return "auth", "must be mil or xor"
	case !imsi.MatchString(r.IMSI):
		return "imsi", "must contain exactly 15 digits"
	case !hex32.MatchString(r.Key):
		return "key", "must contain exactly 32 hexadecimal characters"
	case r.OPType != "op" && r.OPType != "opc":
		return "op_type", "must be op or opc"
	case !hex32.MatchString(r.OPValue):
		return r.OPType, "must contain exactly 32 hexadecimal characters"
	case !hex4.MatchString(r.AMF):
		return "amf", "must contain exactly 4 hexadecimal characters"
	case !hex12.MatchString(r.SQN):
		return "sqn", "must contain exactly 12 hexadecimal characters"
	case r.QCI < 1 || r.QCI > 9:
		return "qci", "must be between 1 and 9"
	case r.IPAlloc == "dynamic":
		return "", ""
	case !isIPv4(r.IPAlloc):
		return "ip_alloc", "must be dynamic or an IPv4 address"
	case !allowExistingStatic:
		return "ip_alloc", "this API accepts dynamic allocation only; existing static values are read-only"
	default:
		return "", ""
	}
}

func Find(records []Record, wanted string) (int, bool) {
	for i := range records {
		if records[i].IMSI == wanted {
			return i, true
		}
	}
	return -1, false
}

func Create(path string, maxBytes int64, record Record) error {
	records, err := Load(path, maxBytes)
	if err != nil {
		return err
	}
	if _, ok := Find(records, record.IMSI); ok {
		return ErrDuplicateIMSI
	}
	for _, existing := range records {
		if existing.Name == record.Name {
			return ErrDuplicateName
		}
	}
	if len(records) >= MaxRecords {
		return errors.New("subscriber database exceeds record limit")
	}
	records = append(records, record)
	return WriteAtomic(path, records)
}

func UpdateMetadata(path string, maxBytes int64, wanted string, nameValue *string, qciValue *int, ipAllocValue *string) (Record, error) {
	records, err := Load(path, maxBytes)
	if err != nil {
		return Record{}, err
	}
	idx, ok := Find(records, wanted)
	if !ok {
		return Record{}, ErrNotFound
	}
	next := records[idx]
	if nameValue != nil {
		next.Name = strings.TrimSpace(*nameValue)
	}
	if qciValue != nil {
		next.QCI = *qciValue
	}
	if ipAllocValue != nil {
		next.IPAlloc = strings.ToLower(strings.TrimSpace(*ipAllocValue))
	}
	// Existing static rows remain readable/editable without weakening the
	// creation rule; changing ip_alloc itself accepts dynamic allocation only.
	if field, reason := validateRecord(next, ipAllocValue == nil); field != "" {
		return Record{}, fmt.Errorf("invalid %s: %s", field, reason)
	}
	for i, existing := range records {
		if i != idx && existing.Name == next.Name {
			return Record{}, ErrDuplicateName
		}
	}
	records[idx] = next
	if err := WriteAtomic(path, records); err != nil {
		return Record{}, err
	}
	return next, nil
}

func Delete(path string, maxBytes int64, wanted string) error {
	records, err := Load(path, maxBytes)
	if err != nil {
		return err
	}
	idx, ok := Find(records, wanted)
	if !ok {
		return ErrNotFound
	}
	records = append(records[:idx], records[idx+1:]...)
	return WriteAtomic(path, records)
}

// ReplacePreservingSQN validates a complete replacement and keeps the current
// on-disk SQN for every unchanged IMSI. This prevents a credential rotation
// upload from rolling the EPC-owned sequence number backwards.
func ReplacePreservingSQN(path string, maxBytes int64, uploaded []byte) (int, error) {
	incoming, err := Parse(strings.NewReader(string(uploaded)), maxBytes)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrIncomingInvalid, err)
	}
	if len(incoming) == 0 {
		return 0, fmt.Errorf("%w: at least one subscriber row is required", ErrIncomingInvalid)
	}
	current, err := Load(path, maxBytes)
	if err != nil {
		return 0, err
	}
	currentSQN := make(map[string]string, len(current))
	currentIP := make(map[string]string, len(current))
	for _, r := range current {
		currentSQN[r.IMSI] = r.SQN
		currentIP[r.IMSI] = r.IPAlloc
	}
	for i := range incoming {
		if incoming[i].IPAlloc != "dynamic" && currentIP[incoming[i].IMSI] != incoming[i].IPAlloc {
			return 0, fmt.Errorf("%w: this API accepts dynamic allocation only; existing static values are read-only", ErrIncomingInvalid)
		}
		if sqn, ok := currentSQN[incoming[i].IMSI]; ok {
			incoming[i].SQN = sqn
		}
	}
	if err := WriteAtomic(path, incoming); err != nil {
		return 0, err
	}
	return len(incoming), nil
}

func WriteAtomic(path string, records []Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".subscribers-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()
	w := csv.NewWriter(tmp)
	for _, r := range records {
		w.Write([]string{r.Name, r.Auth, r.IMSI, r.Key, r.OPType, r.OPValue, r.AMF, r.SQN, strconv.Itoa(r.QCI), r.IPAlloc})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func isIPv4(value string) bool {
	addr, err := netip.ParseAddr(value)
	return err == nil && addr.Is4()
}
