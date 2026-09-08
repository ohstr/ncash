package dial

import (
	"testing"

	"github.com/ohstr/nmilat/nipcash"
	"github.com/ohstr/nmilat/nipcw"
)

func TestSniff_NWCURI(t *testing.T) {
	if got := Sniff("nostr+walletconnect://abc?relay=wss://relay.example&secret=xyz"); got != KindNWCURI {
		t.Errorf("Sniff() = %v, want KindNWCURI", got)
	}
}

func TestSniff_CashToken(t *testing.T) {
	token, err := nipcash.Encode(nipcash.Token{
		HRP:          "lokicash",
		WalletPubkey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Secret:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("test setup: Encode() error = %v", err)
	}
	if got := Sniff(token); got != KindCashToken {
		t.Errorf("Sniff(%q) = %v, want KindCashToken", token, got)
	}
}

func TestSniff_CircleHub(t *testing.T) {
	s, err := nipcw.EncodeCircleHubConnection(nipcw.CircleHubConnection{
		WalletPubkey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Secret:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("test setup: EncodeCircleHubConnection() error = %v", err)
	}
	if got := Sniff(s); got != KindCircleHub {
		t.Errorf("Sniff(%q) = %v, want KindCircleHub", s, got)
	}
}

func TestSniff_CashHub(t *testing.T) {
	s, err := nipcash.EncodeCashHubConnection(nipcash.CashHubConnection{
		WalletPubkey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Secret:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("test setup: EncodeCashHubConnection() error = %v", err)
	}
	if got := Sniff(s); got != KindCashHub {
		t.Errorf("Sniff(%q) = %v, want KindCashHub", s, got)
	}
}

func TestSniff_Unknown(t *testing.T) {
	tests := []string{
		"not a connection string at all",
		"",
		"http://example.com",
		"1234567890",
	}
	for _, in := range tests {
		if got := Sniff(in); got != KindUnknown {
			t.Errorf("Sniff(%q) = %v, want KindUnknown", in, got)
		}
	}
}
