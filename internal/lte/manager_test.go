package lte

import "testing"

func TestStartParams_Validate(t *testing.T) {
	ok := StartParams{Band: "7", APN: "skygoapn", MCC: "001", MNC: "01", Network: "eth0"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid rejected: %v", err)
	}
	bad := StartParams{Band: "7", APN: "", MCC: "001", MNC: "01", Network: "eth0"}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected incomplete error")
	}
	badMCC := StartParams{Band: "7", APN: "a", MCC: "01", MNC: "01", Network: "eth0"}
	if err := badMCC.Validate(); err == nil {
		t.Fatal("expected mcc error")
	}
	inject := StartParams{Band: "7", APN: "a; rm -rf /", MCC: "001", MNC: "01", Network: "eth0"}
	if err := inject.Validate(); err == nil {
		t.Fatal("expected apn injection error")
	}
}
