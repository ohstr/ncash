package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir_DefaultsUnderUserConfigDir(t *testing.T) {
	SetOverride("")
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	want := filepath.Join(tmp, "cashctl")
	if dir != want {
		t.Errorf("Dir() = %q, want %q", dir, want)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("Dir() did not create the directory: %v", err)
	}
}

func TestDir_OverrideWins(t *testing.T) {
	tmp := t.TempDir()
	override := filepath.Join(tmp, "custom")
	SetOverride(override)
	t.Cleanup(func() { SetOverride("") })

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if dir != override {
		t.Errorf("Dir() = %q, want override %q", dir, override)
	}
}
