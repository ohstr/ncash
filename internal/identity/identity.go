// Package identity manages ncash's local identity — compatible with, but
// not a copy of, ncli's own vault (see ncash-plan.md's "Local wallet layer
// (identity + ledger)"). identity.json under appdir.Dir() stores either a
// reference to an existing ncli vault entry, or ncash's own independently
// generated keypair — never both, never a plaintext copy of a vault
// entry's key.
package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	ncli "github.com/ohstr/ncli/client"

	"github.com/ohstr/ncash/internal/appdir"
)

// Source identifies where Resolve should get the signing key from.
type Source string

const (
	// SourceNcliVault: re-unlock the real ncli vault live on every use —
	// identity.json holds only Npub/Label, never a copied privkey.
	SourceNcliVault Source = "ncli-vault"
	// SourceLocal: ncash's own independently generated keypair, stored
	// directly (plaintext) in identity.json — chmod 600, same handling as
	// a seed file.
	SourceLocal Source = "ncash-local"
)

// Stored is the on-disk shape of identity.json.
type Stored struct {
	Source Source `json:"source"`
	// Npub/Label: set only for SourceNcliVault — which vault entry to
	// re-resolve at use time.
	Npub  string `json:"npub,omitempty"`
	Label string `json:"label,omitempty"`
	// PrivHex: set only for SourceLocal.
	PrivHex string `json:"priv_hex,omitempty"`
}

func path() (string, error) {
	dir, err := appdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.json"), nil
}

// Exists reports whether an identity has been configured yet.
func Exists() (bool, error) {
	p, err := path()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ErrNotConfigured is returned by Load when no identity has been set up
// yet — callers should either check Exists first, or surface this as a
// "run `ncash init`" usage error.
var ErrNotConfigured = errors.New("no identity configured yet — run `ncash init` first")

// Load reads the stored identity reference.
func Load() (*Stored, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotConfigured
		}
		return nil, err
	}
	var s Stored
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("identity.json is corrupt: %w", err)
	}
	return &s, nil
}

// SaveNcliVaultRef records a reference to an existing ncli vault entry —
// never a copied privkey.
func SaveNcliVaultRef(npub, label string) error {
	return save(&Stored{Source: SourceNcliVault, Npub: npub, Label: label})
}

// GenerateAndSaveLocal generates a brand-new keypair via ncli's own
// client.GenerateIdentity (the same generator ncli's own `ncli id` uses —
// just persisted under ncash's own identity.json instead of ncli's vault)
// and stores it directly. Returns the new identity's npub.
func GenerateAndSaveLocal() (npub string, err error) {
	id, err := ncli.GenerateIdentity()
	if err != nil {
		return "", fmt.Errorf("failed to generate identity: %w", err)
	}
	if err := save(&Stored{Source: SourceLocal, PrivHex: id.PrivKeyHex}); err != nil {
		return "", err
	}
	return id.Npub, nil
}

func save(s *Stored) error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// 0600: identity.json can hold a plaintext privkey (SourceLocal) — same
	// handling as a seed file, matching ledger.json's own bearer-secret
	// handling (ncash-plan.md's "Local wallet layer").
	return os.WriteFile(p, data, 0600)
}

// PasswordPrompt resolves the ncli vault password when Resolve needs to
// unlock it — sourced from NCLI_VAULT_PASSWORD for non-interactive/agentic
// use, or an interactive prompt otherwise, exactly as ncli's own
// keyresolve.ResolveVaultPassword does. Left to the caller (cmd/) rather
// than baked into this package, so this package needs no terminal I/O of
// its own and stays trivially unit-testable.
type PasswordPrompt func() (string, error)

// Resolve returns the raw private key hex for the stored identity — the
// default `--as pubkey:<privkey>` credential everywhere in ncash unless a
// command overrides it. For SourceNcliVault, this re-unlocks the real
// vault live via promptPassword; promptPassword is never called for
// SourceLocal.
func Resolve(promptPassword PasswordPrompt) (string, error) {
	s, err := Load()
	if err != nil {
		return "", err
	}
	switch s.Source {
	case SourceLocal:
		return s.PrivHex, nil
	case SourceNcliVault:
		entry, found, err := ncli.FindVaultEntry(s.Npub)
		if err != nil {
			return "", fmt.Errorf("failed to look up ncli vault entry: %w", err)
		}
		if !found {
			return "", fmt.Errorf("identity references ncli vault entry %q, but it's no longer in the vault", s.Label)
		}
		password, err := promptPassword()
		if err != nil {
			return "", err
		}
		vaultPrivKeyHex, err := ncli.UnlockVaultIdentity(password)
		if err != nil {
			return "", fmt.Errorf("failed to unlock ncli vault: %w", err)
		}
		return ncli.DecryptVaultEntry(vaultPrivKeyHex, *entry)
	default:
		return "", fmt.Errorf("identity.json has an unknown source %q", s.Source)
	}
}
