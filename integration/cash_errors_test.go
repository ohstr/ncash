//go:build integration

package integration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ohstr/nmilat/nipcash"
	"github.com/ohstr/nmilat/nipcw"
)

// These tests need no admin API and no network: they fabricate
// syntactically valid (but never-dialed) cash/circle/hub connection
// strings locally via nmilat's own Encode functions, then drive the
// compiled binary through error paths that fail before any network call
// is ever made (dial.Sniff-based mis-paste detection, and the
// no-wallet-configured check) — deterministic, always-runnable, no
// config.local.yaml required.

func fakeHex32(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func fakeCashHubConnection(t *testing.T) string {
	t.Helper()
	tok, err := nipcash.EncodeCashHubConnection(nipcash.CashHubConnection{
		WalletPubkey: fakeHex32(t),
		Secret:       fakeHex32(t),
		RelayURLs:    []string{"wss://fake.invalid"},
	})
	if err != nil {
		t.Fatalf("encode fake cash hub connection: %v", err)
	}
	return tok
}

func fakeCircleHubConnection(t *testing.T) string {
	t.Helper()
	tok, err := nipcw.EncodeCircleHubConnection(nipcw.CircleHubConnection{
		WalletPubkey: fakeHex32(t),
		Secret:       fakeHex32(t),
		RelayURLs:    []string{"wss://fake.invalid"},
	})
	if err != nil {
		t.Fatalf("encode fake circle hub connection: %v", err)
	}
	return tok
}

func TestCashReceive_MisPasteCashHub(t *testing.T) {
	f := newFixture(t)
	f.mustJSON("wallet", "init")
	res := f.run("receive", fakeCashHubConnection(t))
	if res.ExitCode != 3 {
		t.Fatalf("receive cashhub1...: exit = %d, want 3 (invalid_input)\nstderr: %s", res.ExitCode, res.Stderr)
	}
	if !jsonErrorContains(t, res.Stderr, "invalid_input", "Cash Hub") {
		t.Errorf("unexpected error body: %s", res.Stderr)
	}
}

func TestCashReceive_MisPasteCircleHub(t *testing.T) {
	f := newFixture(t)
	f.mustJSON("wallet", "init")
	res := f.run("receive", fakeCircleHubConnection(t))
	if res.ExitCode != 3 {
		t.Fatalf("receive circlehub1...: exit = %d, want 3 (invalid_input)\nstderr: %s", res.ExitCode, res.Stderr)
	}
	if !jsonErrorContains(t, res.Stderr, "invalid_input", "join") {
		t.Errorf("unexpected error body: %s", res.Stderr)
	}
}

func TestCircleJoin_MisPasteCashHub(t *testing.T) {
	f := newFixture(t)
	f.mustJSON("wallet", "init")
	res := f.run("join", "--hub", fakeCashHubConnection(t))
	if res.ExitCode != 3 {
		t.Fatalf("join --hub cashhub1...: exit = %d, want 3 (invalid_input)\nstderr: %s", res.ExitCode, res.Stderr)
	}
	if !jsonErrorContains(t, res.Stderr, "invalid_input", "Cash Hub") {
		t.Errorf("unexpected error body: %s", res.Stderr)
	}
}

// jsonErrorContains parses stderr as cashctl's --json error shape and checks
// its code and that its error message contains substr.
func jsonErrorContains(t *testing.T, stderr, wantCode, substr string) bool {
	t.Helper()
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal([]byte(stderr), &body); err != nil {
		t.Errorf("decode stderr JSON: %v: %s", err, stderr)
		return false
	}
	if body.Code != wantCode {
		t.Errorf("code = %q, want %q", body.Code, wantCode)
		return false
	}
	return strings.Contains(body.Error, substr)
}
