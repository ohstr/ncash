package credential

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohstr/nmilat/nipIC"
	"github.com/ohstr/nmilat/utils"
)

func randomPrivKeyHex(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}

func TestParseCash_Pubkey(t *testing.T) {
	priv := randomPrivKeyHex(t)
	cred, err := ParseCash("pubkey:" + priv)
	if err != nil {
		t.Fatalf("ParseCash() error = %v", err)
	}
	if cred == nil {
		t.Fatal("ParseCash() returned nil credential")
	}
}

func TestParseCash_Bearer(t *testing.T) {
	cred, err := ParseCash("bearer:some-secret")
	if err != nil {
		t.Fatalf("ParseCash() error = %v", err)
	}
	if cred == nil {
		t.Fatal("ParseCash() returned nil credential")
	}
}

func TestParseCash_Errors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"no colon", "pubkey-no-colon"},
		{"empty pubkey", "pubkey:"},
		{"empty bearer", "bearer:"},
		{"unknown kind", "carrier-pigeon:abc"},
		{"connection-key wrong field count", "connection-key:abc,discord"},
		{"connection-key empty field", "connection-key:abc,,482910,file.json"},
		{"connection-key missing file", "connection-key:abc,discord,482910,/does/not/exist.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseCash(tt.in); err == nil {
				t.Errorf("ParseCash(%q) = nil error, want an error", tt.in)
			}
		})
	}
}

func TestParseCash_ConnectionKey_ValidAttestation(t *testing.T) {
	iaPriv := randomPrivKeyHex(t)
	userPriv := randomPrivKeyHex(t)
	userPub, err := utils.GetPublicKey(userPriv)
	if err != nil {
		t.Fatalf("GetPublicKey() error = %v", err)
	}
	connKey := nipIC.NewConnectionKey("discord", "482910")

	attestationEvent, err := nipIC.NewAttestation(nipIC.AttestationParams{
		PrivateKey:    iaPriv,
		ConnectionKey: connKey,
		UserPubkey:    userPub,
		Platform:      "discord",
		Evidence: nipIC.Evidence{
			Platform: "discord", UserID: "482910", Username: "someone", VerifiedAt: 1720000000,
		},
	})
	if err != nil {
		t.Fatalf("NewAttestation() error = %v", err)
	}

	data, err := json.Marshal(attestationEvent)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	file := filepath.Join(t.TempDir(), "attestation.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cred, err := ParseCash("connection-key:" + userPriv + ",discord,482910," + file)
	if err != nil {
		t.Fatalf("ParseCash() error = %v", err)
	}
	if cred == nil {
		t.Fatal("ParseCash() returned nil credential")
	}
}

func TestParseCash_ConnectionKey_MalformedAttestationFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(file, []byte("not json"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := ParseCash("connection-key:priv,discord,482910," + file); err == nil {
		t.Error("expected an error parsing a malformed attestation file")
	}
}

func TestParseCircle_Pubkey(t *testing.T) {
	priv := randomPrivKeyHex(t)
	if _, err := ParseCircle("pubkey:" + priv); err != nil {
		t.Fatalf("ParseCircle() error = %v", err)
	}
}

func TestParseCircle_RejectsNonPubkeyModes(t *testing.T) {
	tests := []string{"bearer:secret", "connection-key:a,b,c,d", "pubkey:", "no-colon-at-all"}
	for _, in := range tests {
		if _, err := ParseCircle(in); err == nil {
			t.Errorf("ParseCircle(%q) = nil error, want an error (NIP-CW has only pubkey mode)", in)
		}
	}
}

func TestParseTarget_Pubkey(t *testing.T) {
	priv := randomPrivKeyHex(t)
	pub, err := utils.GetPublicKey(priv)
	if err != nil {
		t.Fatalf("GetPublicKey() error = %v", err)
	}
	target, err := ParseTarget("pubkey:" + pub)
	if err != nil {
		t.Fatalf("ParseTarget() error = %v", err)
	}
	if target == nil {
		t.Fatal("ParseTarget() returned nil target")
	}
}

func TestParseTarget_Connection(t *testing.T) {
	target, err := ParseTarget("connection:discord:482910:deadbeef")
	if err != nil {
		t.Fatalf("ParseTarget() error = %v", err)
	}
	if target == nil {
		t.Fatal("ParseTarget() returned nil target")
	}
}

func TestParseTarget_BearerTarget_GeneratesFreshSecretEachTime(t *testing.T) {
	t1, err := ParseTarget("bearer-target")
	if err != nil {
		t.Fatalf("ParseTarget() error = %v", err)
	}
	t2, err := ParseTarget("bearer-target")
	if err != nil {
		t.Fatalf("ParseTarget() error = %v", err)
	}

	bt1, ok := t1.(interface{ Secret() string })
	if !ok {
		t.Fatal("bearer-target result does not expose Secret()")
	}
	bt2 := t2.(interface{ Secret() string })

	if bt1.Secret() == "" {
		t.Error("Secret() is empty")
	}
	if bt1.Secret() == bt2.Secret() {
		t.Error("two bearer-target calls produced the same secret — should be fresh each time")
	}
}

func TestParseTarget_Errors(t *testing.T) {
	tests := []string{
		"no-colon",
		"pubkey:",
		"connection:only-one-field",
		"connection:discord:482910",
		"unknown-kind:x",
	}
	for _, in := range tests {
		if _, err := ParseTarget(in); err == nil {
			t.Errorf("ParseTarget(%q) = nil error, want an error", in)
		}
	}
}
