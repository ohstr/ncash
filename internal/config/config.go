// Package config manages cashctl's multi-wallet inventory: named connections
// (both ones cashctl produced itself — a join result, a redeem destination —
// and foreign ones added via `connect add`) plus a single default pointer
// (see cashctl-plan.md's "Multi-wallet inventory & defaults").
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ohstr/cashctl/internal/appdir"
)

// Connection is one named wallet/hub connection cashctl knows about — either
// a raw nostr+walletconnect:// URI, or a bech32 string
// (cashhub1.../circlehub1.../lokicash1...) — stored exactly as given.
type Connection struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	AddedAt string `json:"added_at"`

	// LastKnownBalanceMloki/LastKnownBalanceAt cache the most recent
	// successful live get_balance result — the only way to show a figure
	// for an expired wallet at all, since get_balance itself is one of the
	// money-moving scopes an expired wallet rejects (only get_info/
	// get_budget survive expiry). `cashctl balance` falls back to this,
	// flagged as stranded, when a live call fails specifically with
	// EXPIRED.
	LastKnownBalanceMloki *int64 `json:"last_known_balance_mloki,omitempty"`
	LastKnownBalanceAt    string `json:"last_known_balance_at,omitempty"`
}

// Store is the on-disk shape of connections.json.
type Store struct {
	Connections []Connection `json:"connections,omitempty"`
	Default     string       `json:"default,omitempty"`
}

func path() (string, error) {
	dir, err := appdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "connections.json"), nil
}

// Load reads connections.json, returning an empty (not nil) *Store if the
// file doesn't exist yet — a fresh install has no connections, not an
// error condition.
func Load() (*Store, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("connections.json is corrupt: %w", err)
	}
	return &s, nil
}

// Save persists s. 0600: a connection's Value is frequently a secret-
// bearing pairing URI, not just a public identifier.
func (s *Store) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// Find looks up a connection by name.
func (s *Store) Find(name string) (*Connection, bool) {
	for i := range s.Connections {
		if s.Connections[i].Name == name {
			return &s.Connections[i], true
		}
	}
	return nil, false
}

// ErrDuplicateName is returned by Add when name is already taken.
var ErrDuplicateName = errors.New("a connection with that name already exists")

// Add records a new connection under name. Does not touch Default — see
// SetDefault and cashctl-plan.md's "registering a new wallet never silently
// changes the default" rule; callers decide separately whether to also
// call SetDefault (e.g. after prompting the user, or automatically for the
// very first connection — see IsEmpty).
func (s *Store) Add(name, value string) error {
	if _, ok := s.Find(name); ok {
		return ErrDuplicateName
	}
	s.Connections = append(s.Connections, Connection{Name: name, Value: value, AddedAt: nowRFC3339()})
	return nil
}

// Remove deletes the named connection, and clears Default if it pointed at
// the one being removed.
func (s *Store) Remove(name string) bool {
	for i := range s.Connections {
		if s.Connections[i].Name == name {
			s.Connections = append(s.Connections[:i], s.Connections[i+1:]...)
			if s.Default == name {
				s.Default = ""
			}
			return true
		}
	}
	return false
}

// IsEmpty reports whether this is a fresh inventory with no connections
// yet — the one case cashctl-plan.md's default-pointer rule inverts from "ask
// [y/N]" to "ask [Y/n]" (a first wallet has no existing default to
// protect).
func (s *Store) IsEmpty() bool {
	return len(s.Connections) == 0
}

// SetDefault points Default at name. Returns an error if name isn't a
// known connection.
func (s *Store) SetDefault(name string) error {
	if _, ok := s.Find(name); !ok {
		return fmt.Errorf("no connection named %q", name)
	}
	s.Default = name
	return nil
}

// SetLastKnownBalance caches a successful live get_balance result against
// the named connection (see Connection's own doc comment).
func (s *Store) SetLastKnownBalance(name string, mloki int64) {
	if c, ok := s.Find(name); ok {
		c.LastKnownBalanceMloki = &mloki
		c.LastKnownBalanceAt = nowRFC3339()
	}
}

// DefaultConnection returns the current default connection, or false if
// none is set.
func (s *Store) DefaultConnection() (*Connection, bool) {
	if s.Default == "" {
		return nil, false
	}
	return s.Find(s.Default)
}

// slugPattern matches characters SuggestName keeps from a hint — letters,
// digits, and hyphens; everything else (spaces, punctuation from a
// human-entered label like "Ada's Family Circle") is dropped rather than
// erroring, since a hint is advisory, not user input to validate.
var slugPattern = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

// SuggestName returns an available "<prefix>:<slug>" name derived from
// hint (e.g. a decoded circlehub1... token's own Label field), falling
// back to "<prefix>:<n>" if hint is empty, slugifies to nothing, or
// collides with an existing connection.
func (s *Store) SuggestName(prefix, hint string) string {
	slug := strings.ToLower(strings.Trim(slugPattern.ReplaceAllString(hint, "-"), "-"))
	if slug != "" {
		candidate := prefix + ":" + slug
		if _, ok := s.Find(candidate); !ok {
			return candidate
		}
	}
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s:%d", prefix, n)
		if _, ok := s.Find(candidate); !ok {
			return candidate
		}
	}
}
