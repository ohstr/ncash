package ledger

import (
	"errors"
	"os"
	"testing"

	"github.com/ohstr/cashctl/internal/appdir"
)

func withTempConfigDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	appdir.SetOverride(tmp)
	t.Cleanup(func() { appdir.SetOverride("") })
}

func TestLoad_EmptyOnFreshInstall(t *testing.T) {
	withTempConfigDir(t)

	l, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(l.Entries) != 0 || len(l.Held()) != 0 {
		t.Error("fresh ledger is not empty")
	}
}

func TestAdd_GeneratesIDAndSetsDefaults(t *testing.T) {
	l := &Ledger{}
	entry, err := l.Add(Entry{Token: "lokicash1abc", WalletPubkey: "wp", Secret: "s"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if entry.ID == "" {
		t.Error("Add() did not assign an ID")
	}
	if entry.Status != StatusHeld {
		t.Errorf("Status = %q, want %q", entry.Status, StatusHeld)
	}
	if entry.Verified {
		t.Error("Verified = true on a freshly received token, want false")
	}
	if entry.ReceivedAt == "" {
		t.Error("ReceivedAt was not set")
	}
}

func TestAdd_RejectsDuplicateToken(t *testing.T) {
	l := &Ledger{}
	if _, err := l.Add(Entry{Token: "lokicash1abc"}); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	if _, err := l.Add(Entry{Token: "lokicash1abc"}); !errors.Is(err, ErrAlreadyHeld) {
		t.Errorf("second Add() error = %v, want ErrAlreadyHeld", err)
	}
}

func TestFindByToken(t *testing.T) {
	l := &Ledger{}
	added, _ := l.Add(Entry{Token: "lokicash1abc"})
	found, ok := l.FindByToken("lokicash1abc")
	if !ok {
		t.Fatal("FindByToken() = not found")
	}
	if found.ID != added.ID {
		t.Errorf("FindByToken() ID = %q, want %q", found.ID, added.ID)
	}
	if _, ok := l.FindByToken("lokicash1doesnotexist"); ok {
		t.Error("FindByToken() found a token that was never added")
	}
}

func TestHeld_ExcludesNonHeldStatuses(t *testing.T) {
	l := &Ledger{}
	a, _ := l.Add(Entry{Token: "t1"})
	_, _ = l.Add(Entry{Token: "t2"})
	if err := l.SetStatus(a.ID, StatusRedeemed); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}

	held := l.Held()
	if len(held) != 1 {
		t.Fatalf("Held() returned %d entries, want 1", len(held))
	}
	if held[0].Token != "t2" {
		t.Errorf("Held()[0].Token = %q, want %q", held[0].Token, "t2")
	}
}

func TestSetStatus_UnknownIDErrors(t *testing.T) {
	l := &Ledger{}
	if err := l.SetStatus("tok-doesnotexist", StatusRedeemed); err == nil {
		t.Error("expected an error setting status on an unknown entry")
	}
}

func TestSetVerified(t *testing.T) {
	l := &Ledger{}
	e, _ := l.Add(Entry{Token: "t1"})
	if e.Verified {
		t.Fatal("precondition failed: entry starts verified")
	}
	if err := l.SetVerified(e.ID, true); err != nil {
		t.Fatalf("SetVerified() error = %v", err)
	}
	found, _ := l.Find(e.ID)
	if !found.Verified {
		t.Error("Verified was not updated")
	}
}

func TestNewID_NoCollisions(t *testing.T) {
	l := &Ledger{}
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		e, err := l.Add(Entry{Token: "t" + string(rune('a'+i%26)) + string(rune(i))})
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if seen[e.ID] {
			t.Fatalf("duplicate ID generated: %q", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestAppendHistory(t *testing.T) {
	l := &Ledger{}
	l.AppendHistory("receive", "received 20000 mloki")
	if len(l.History) != 1 {
		t.Fatalf("History length = %d, want 1", len(l.History))
	}
	if l.History[0].Action != "receive" || l.History[0].At == "" {
		t.Errorf("History[0] = %+v, unexpected", l.History[0])
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	withTempConfigDir(t)

	l, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	amount := uint64(20000)
	identityRequired := true
	if _, err := l.Add(Entry{
		Token: "lokicash1abc", WalletPubkey: "wp", Secret: "s",
		RelayURLs: []string{"wss://relay.example"}, AmountMillis: &amount,
		IdentityRequired: &identityRequired,
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	l.AppendHistory("receive", "test")
	if err := l.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load() (reloaded) error = %v", err)
	}
	if len(reloaded.Entries) != 1 || reloaded.Entries[0].Token != "lokicash1abc" {
		t.Fatalf("reloaded entries = %+v, unexpected", reloaded.Entries)
	}
	if reloaded.Entries[0].AmountMillis == nil || *reloaded.Entries[0].AmountMillis != amount {
		t.Errorf("AmountMillis did not round-trip")
	}
	if len(reloaded.History) != 1 {
		t.Errorf("History did not round-trip")
	}
}

func TestSave_FilePermissions(t *testing.T) {
	withTempConfigDir(t)

	l, _ := Load()
	_, _ = l.Add(Entry{Token: "lokicash1bearer", Secret: "a-bearer-secret-is-money"})
	if err := l.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	p, err := path()
	if err != nil {
		t.Fatalf("path() error = %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("ledger.json permissions = %o, want 0600", perm)
	}
}
