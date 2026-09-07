package subscriber

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const validRow = "ue0,mil,001010123456789,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic\n"

func TestParseStrictAndRedacted(t *testing.T) {
	records, err := Parse(strings.NewReader(validRow), 4096)
	if err != nil || len(records) != 1 {
		t.Fatalf("parse: records=%d err=%v", len(records), err)
	}
	public := records[0].Redacted()
	if !public.Authorized || public.Active != nil || public.CredentialStatus != "configured" {
		t.Fatalf("unexpected public view: %+v", public)
	}
	for _, bad := range []string{
		"ue0,mil,001,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic\n",
		"ue0,mil,001010123456789,short,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic\n",
		validRow + validRow,
	} {
		if _, err := Parse(strings.NewReader(bad), 4096); err == nil {
			t.Fatalf("accepted invalid database: %q", bad)
		}
	}
}

func TestReplacePreservesCurrentSQN(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user_db.csv")
	if err := os.WriteFile(path, []byte(validRow), 0o644); err != nil {
		t.Fatal(err)
	}
	upload := strings.Replace(validRow, "000000001234", "00000000ffff", 1)
	upload = strings.Replace(upload, "00112233445566778899aabbccddeeff", "ffeeddccbbaa99887766554433221100", 1)
	if _, err := ReplacePreservingSQN(path, 4096, []byte(upload)); err != nil {
		t.Fatal(err)
	}
	records, err := Load(path, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].SQN != "000000001234" {
		t.Fatalf("SQN rolled back: %q", records[0].SQN)
	}
	if records[0].Key != "ffeeddccbbaa99887766554433221100" {
		t.Fatalf("credential rotation was not applied: %q", records[0].Key)
	}
}

func TestMutationsPreserveSecretsAndRejectStaticCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user_db.csv")
	records, err := Parse(strings.NewReader(validRow), 4096)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, records); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("subscriber credential database mode=%o, want 600", info.Mode().Perm())
		}
	}
	name := "phone"
	qci := 9
	updated, err := UpdateMetadata(path, 4096, records[0].IMSI, &name, &qci, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Key != records[0].Key || updated.SQN != records[0].SQN || updated.Name != name {
		t.Fatalf("metadata update changed protected fields: %+v", updated)
	}
	static := "172.16.0.10"
	if _, err := UpdateMetadata(path, 4096, records[0].IMSI, nil, nil, &static); err == nil {
		t.Fatal("static update accepted without a pool contract")
	}
	if err := Delete(path, 4096, records[0].IMSI); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, 4096)
	if err != nil || len(got) != 0 {
		t.Fatalf("delete: records=%d err=%v", len(got), err)
	}
}

func TestParseRejectsDuplicateNameAndMappedIPv4(t *testing.T) {
	second := strings.Replace(validRow, "ue0", "ue1", 1)
	second = strings.Replace(second, "001010123456789", "001010123456788", 1)
	duplicateName := validRow + strings.Replace(second, "ue1", "ue0", 1)
	if _, err := Parse(strings.NewReader(duplicateName), 4096); err == nil {
		t.Fatal("duplicate subscriber name accepted")
	}
	mapped := strings.Replace(validRow, "dynamic", "::ffff:172.16.0.2", 1)
	if _, err := Parse(strings.NewReader(mapped), 4096); err == nil {
		t.Fatal("IPv4-mapped IPv6 accepted as static IPv4")
	}
}
