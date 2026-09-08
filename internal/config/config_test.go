package config

import (
	"os"
	"testing"

	"github.com/ohstr/ncash/internal/appdir"
)

func withTempConfigDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	appdir.SetOverride(tmp)
	t.Cleanup(func() { appdir.SetOverride("") })
}

func TestLoad_EmptyOnFreshInstall(t *testing.T) {
	withTempConfigDir(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !s.IsEmpty() {
		t.Error("Load() on a fresh install is not empty")
	}
	if _, ok := s.DefaultConnection(); ok {
		t.Error("DefaultConnection() on a fresh install returned ok=true")
	}
}

func TestAddAndSaveRoundTrip(t *testing.T) {
	withTempConfigDir(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := s.Add("lightning:default", "nostr+walletconnect://abc?relay=wss://relay.example&secret=xyz"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load() (reloaded) error = %v", err)
	}
	c, ok := reloaded.Find("lightning:default")
	if !ok {
		t.Fatal("Find() after reload = not found")
	}
	if c.Value != "nostr+walletconnect://abc?relay=wss://relay.example&secret=xyz" {
		t.Errorf("Value = %q, unexpected", c.Value)
	}
}

func TestAdd_DuplicateNameRejected(t *testing.T) {
	s := &Store{}
	if err := s.Add("a", "v1"); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	if err := s.Add("a", "v2"); err == nil {
		t.Fatal("expected ErrDuplicateName, got nil")
	} else if err != ErrDuplicateName {
		t.Errorf("error = %v, want ErrDuplicateName", err)
	}
}

func TestRemove_ClearsDefaultIfItWasTheOneRemoved(t *testing.T) {
	s := &Store{}
	_ = s.Add("a", "v1")
	_ = s.SetDefault("a")

	if !s.Remove("a") {
		t.Fatal("Remove() = false, want true")
	}
	if _, ok := s.DefaultConnection(); ok {
		t.Error("DefaultConnection() still resolves after removing the default connection")
	}
	if s.Default != "" {
		t.Errorf("Default = %q, want cleared", s.Default)
	}
}

func TestRemove_UnrelatedDefaultUntouched(t *testing.T) {
	s := &Store{}
	_ = s.Add("a", "v1")
	_ = s.Add("b", "v2")
	_ = s.SetDefault("a")

	s.Remove("b")
	if s.Default != "a" {
		t.Errorf("Default = %q, want unchanged %q", s.Default, "a")
	}
}

func TestSetDefault_UnknownNameErrors(t *testing.T) {
	s := &Store{}
	if err := s.SetDefault("does-not-exist"); err == nil {
		t.Fatal("expected an error setting default to an unknown connection")
	}
}

func TestIsEmpty(t *testing.T) {
	s := &Store{}
	if !s.IsEmpty() {
		t.Error("IsEmpty() = false on a brand new Store")
	}
	_ = s.Add("a", "v")
	if s.IsEmpty() {
		t.Error("IsEmpty() = true after Add")
	}
}

func TestSuggestName_UsesHintWhenAvailable(t *testing.T) {
	s := &Store{}
	got := s.SuggestName("circle", "Ada's Family Circle")
	want := "circle:ada-s-family-circle"
	if got != want {
		t.Errorf("SuggestName() = %q, want %q", got, want)
	}
}

func TestSuggestName_FallsBackToNumberOnEmptyHint(t *testing.T) {
	s := &Store{}
	got := s.SuggestName("circle", "")
	if got != "circle:1" {
		t.Errorf("SuggestName() = %q, want %q", got, "circle:1")
	}
}

func TestSuggestName_AvoidsCollision(t *testing.T) {
	s := &Store{}
	_ = s.Add("circle:family", "v1")
	got := s.SuggestName("circle", "Family")
	if got == "circle:family" {
		t.Errorf("SuggestName() collided with an existing name: %q", got)
	}
	if got != "circle:1" {
		t.Errorf("SuggestName() = %q, want fallback %q", got, "circle:1")
	}
}

func TestSuggestName_NumericFallbackAvoidsCollision(t *testing.T) {
	s := &Store{}
	_ = s.Add("circle:1", "v1")
	_ = s.Add("circle:2", "v2")
	got := s.SuggestName("circle", "")
	if got != "circle:3" {
		t.Errorf("SuggestName() = %q, want %q", got, "circle:3")
	}
}

func TestSave_FilePermissions(t *testing.T) {
	withTempConfigDir(t)

	s, _ := Load()
	_ = s.Add("a", "secret-bearing-value")
	if err := s.Save(); err != nil {
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
		t.Errorf("connections.json permissions = %o, want 0600", perm)
	}
}
