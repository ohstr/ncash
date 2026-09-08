// Package ledger tracks cash tokens ncash has received or produced for
// itself, and a chronological log of local actions (ncash-plan.md's
// "Ledger entries store what's needed to act again without re-asking the
// user").
package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ohstr/ncash/internal/appdir"
)

// Status values a held token moves through.
const (
	StatusHeld         = "held"
	StatusRedeemed     = "redeemed"
	StatusTransferred  = "transferred"
	StatusConsolidated = "consolidated"
)

// Entry is one cash token ncash knows about. WalletPubkey/Secret/RelayURLs/
// IdentityRequired/AmountMillis are decoded straight from Token (see
// nipcash.Decode) — cached here for display convenience, not as a separate
// source of truth. The connection-key fields are the one thing that can't
// be recovered from the token bytes alone (see ncash-plan.md): a
// *reference* (attestation event ID + IA pubkey), never a cached copy of
// the attestation event, so redeeming always re-checks live revocation.
type Entry struct {
	ID           string `json:"id"`
	Token        string `json:"token"`
	WalletPubkey string `json:"wallet_pubkey"`
	// Secret is the token's own type-2 TLV field: the NWC connection
	// secret. It lets ncash dial the wallet (list-recipients, decode,
	// ...) but is NEVER sufficient to redeem/transfer a bearer slice —
	// see BearerSecret's own doc comment (NIP-CASH.md's Redemption
	// Metadata section covers this distinction in full).
	Secret           string   `json:"secret"`
	RelayURLs        []string `json:"relay_urls,omitempty"`
	IdentityRequired *bool    `json:"identity_required,omitempty"`
	AmountMillis     *uint64  `json:"amount_millis,omitempty"`
	ReceivedAt       string   `json:"received_at"`
	Verified         bool     `json:"verified"`
	Status           string   `json:"status"`

	// BearerSecret is the actual spending credential for a bearer-mode
	// token — a value that exists only in mint_cash's own response,
	// returned exactly once, and can never be derived from the token
	// itself (unlike every other cached field on this Entry). Set only if
	// the user supplied `--secret` at receive time; empty otherwise, in
	// which case redeeming/transferring this entry requires an explicit
	// `--as bearer:<secret>` override, the same treatment connection-key
	// mode already gets below.
	BearerSecret string `json:"bearer_secret,omitempty"`

	// Connection-key mode reference — set only when the user has told
	// ncash this token is connection-key-bound (not derivable from the
	// token itself; IdentityRequired only says a proof is needed, not
	// which mode). Empty for pubkey- and bearer-mode tokens.
	ConnectionKeyPlatform   string `json:"connection_key_platform,omitempty"`
	ConnectionKeyExternalID string `json:"connection_key_external_id,omitempty"`
	AttestationEventID      string `json:"attestation_event_id,omitempty"`
	IAPubkey                string `json:"ia_pubkey,omitempty"`
}

// HistoryEntry is one line of ncash's local action log (`ncash wallet
// history`).
type HistoryEntry struct {
	At     string `json:"at"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

// Ledger is the on-disk shape of ledger.json.
type Ledger struct {
	Entries []Entry        `json:"entries,omitempty"`
	History []HistoryEntry `json:"history,omitempty"`
}

func path() (string, error) {
	dir, err := appdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ledger.json"), nil
}

// Load reads ledger.json, returning an empty (not nil) *Ledger if the file
// doesn't exist yet.
func Load() (*Ledger, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Ledger{}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("ledger.json is corrupt: %w", err)
	}
	return &l, nil
}

// Save persists l. 0600: a bearer-mode entry's Secret field is the money —
// see ncash-plan.md's "makes ledger.json as sensitive as a seed file."
func (l *Ledger) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// ErrAlreadyHeld is returned by Add when token has already been recorded.
var ErrAlreadyHeld = errors.New("this token is already in your ledger")

// FindByToken looks up an entry by its original token string — used to
// reject re-receiving the same token twice.
func (l *Ledger) FindByToken(token string) (*Entry, bool) {
	for i := range l.Entries {
		if l.Entries[i].Token == token {
			return &l.Entries[i], true
		}
	}
	return nil, false
}

// Find looks up an entry by its short local ID (e.g. "tok-8e21").
func (l *Ledger) Find(id string) (*Entry, bool) {
	for i := range l.Entries {
		if l.Entries[i].ID == id {
			return &l.Entries[i], true
		}
	}
	return nil, false
}

// Held returns every entry whose Status is StatusHeld — the pool `redeem`/
// `transfer`/`consolidate` auto-pick from when no local ID is given
// explicitly.
func (l *Ledger) Held() []Entry {
	var held []Entry
	for _, e := range l.Entries {
		if e.Status == StatusHeld {
			held = append(held, e)
		}
	}
	return held
}

// Add records a new entry, generating a short local ID and setting
// ReceivedAt/Status/Verified. Rejects a token already in the ledger.
func (l *Ledger) Add(e Entry) (*Entry, error) {
	if _, ok := l.FindByToken(e.Token); ok {
		return nil, ErrAlreadyHeld
	}
	e.ID = l.newID()
	e.ReceivedAt = nowRFC3339()
	e.Status = StatusHeld
	l.Entries = append(l.Entries, e)
	return &l.Entries[len(l.Entries)-1], nil
}

// SetStatus transitions an entry's status (e.g. to StatusRedeemed once
// cash_redeem succeeds) — a redeemed/transferred/consolidated token is
// never removed outright, only marked, so `wallet history`/an audit trail
// stays intact.
func (l *Ledger) SetStatus(id, status string) error {
	for i := range l.Entries {
		if l.Entries[i].ID == id {
			l.Entries[i].Status = status
			return nil
		}
	}
	return fmt.Errorf("no held token %q", id)
}

// SetVerified marks an entry as Hub-cross-checked (see `receive --verify`).
func (l *Ledger) SetVerified(id string, verified bool) error {
	for i := range l.Entries {
		if l.Entries[i].ID == id {
			l.Entries[i].Verified = verified
			return nil
		}
	}
	return fmt.Errorf("no held token %q", id)
}

// AppendHistory adds one line to the local action log.
func (l *Ledger) AppendHistory(action, detail string) {
	l.History = append(l.History, HistoryEntry{At: nowRFC3339(), Action: action, Detail: detail})
}

// newID generates a short, human-typeable local ID in the "tok-xxxx" shape
// shown throughout ncash-plan.md's walkthroughs, retrying on the
// astronomically unlikely collision with an existing entry.
func (l *Ledger) newID() string {
	for {
		var b [2]byte
		_, _ = rand.Read(b[:])
		id := "tok-" + hex.EncodeToString(b[:])
		if _, ok := l.Find(id); !ok {
			return id
		}
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
