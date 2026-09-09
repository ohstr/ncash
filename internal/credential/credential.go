// Package credential parses cashctl's flag-string syntax for identity
// credentials and cash-transfer targets (cashctl-plan.md's Cash command
// tree) into the nipcash/nipcw types the SDK itself expects. This syntax
// is only needed for the override case — acting as/for someone else — the
// default path resolves a held token's credential from the ledger
// directly, never through this parser (see internal/ledger).
package credential

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ohstr/nmilat/nip01"
	"github.com/ohstr/nmilat/nipIC"
	"github.com/ohstr/nmilat/nipcash"
	"github.com/ohstr/nmilat/nipcw"
)

// ParseCash parses a NIP-CASH credential string — one of:
//
//	pubkey:<privkey>
//	connection-key:<privkey>,<platform>,<external-id>,<attestation-file>
//	bearer:<secret>
func ParseCash(s string) (nipcash.Credential, error) {
	prefix, rest, ok := strings.Cut(s, ":")
	if !ok {
		return nil, fmt.Errorf("credential must be pubkey:<privkey>, connection-key:<privkey>,<platform>,<external-id>,<attestation-file>, or bearer:<secret>")
	}
	switch prefix {
	case "pubkey":
		if rest == "" {
			return nil, fmt.Errorf("pubkey: credential is missing a private key")
		}
		return nipcash.BySigning(rest), nil
	case "bearer":
		if rest == "" {
			return nil, fmt.Errorf("bearer: credential is missing a secret")
		}
		return nipcash.BySecret(rest), nil
	case "connection-key":
		privKey, platform, externalID, attestationFile, err := splitConnectionKey(rest)
		if err != nil {
			return nil, err
		}
		attestation, err := loadAttestation(attestationFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load attestation file %q: %w", attestationFile, err)
		}
		return nipcash.BySigningConnectionKey(privKey, nipIC.WebIdentity(platform), externalID, attestation), nil
	default:
		return nil, fmt.Errorf("unknown credential kind %q (want pubkey, connection-key, or bearer)", prefix)
	}
}

// ParseCircle parses a NIP-CW credential string. NIP-CW has only one mode:
//
//	pubkey:<privkey>
func ParseCircle(s string) (nipcw.Credential, error) {
	prefix, rest, ok := strings.Cut(s, ":")
	if !ok || prefix != "pubkey" || rest == "" {
		return nipcw.Credential{}, fmt.Errorf("circle credential must be pubkey:<privkey>")
	}
	return nipcw.BySigning(rest), nil
}

// ParseTarget parses a cash_transfer --to target string — one of:
//
//	pubkey:<hex>
//	connection:<platform>:<external-id>:<ia-pubkey>
//	bearer-target
func ParseTarget(s string) (nipcash.Target, error) {
	if s == "bearer-target" {
		return nipcash.NewBearerTarget(), nil
	}
	prefix, rest, ok := strings.Cut(s, ":")
	if !ok {
		return nil, fmt.Errorf("target must be pubkey:<hex>, connection:<platform>:<external-id>:<ia-pubkey>, or bearer-target")
	}
	switch prefix {
	case "pubkey":
		if rest == "" {
			return nil, fmt.Errorf("pubkey: target is missing a hex pubkey")
		}
		return nipcash.Pubkey(rest), nil
	case "connection":
		parts := strings.Split(rest, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("connection: target needs platform:external-id:ia-pubkey, got %d field(s)", len(parts))
		}
		return nipcash.ConnectionKey(nipIC.WebIdentity(parts[0]), parts[1], parts[2]), nil
	default:
		return nil, fmt.Errorf("unknown target kind %q (want pubkey, connection, or bearer-target)", prefix)
	}
}

func splitConnectionKey(rest string) (privKey, platform, externalID, attestationFile string, err error) {
	parts := strings.Split(rest, ",")
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("connection-key: needs privkey,platform,external-id,attestation-file, got %d field(s)", len(parts))
	}
	for i, p := range parts {
		if p == "" {
			names := []string{"privkey", "platform", "external-id", "attestation-file"}
			return "", "", "", "", fmt.Errorf("connection-key: %s is empty", names[i])
		}
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

func loadAttestation(file string) (*nipIC.Attestation, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var ev nip01.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, fmt.Errorf("not a valid Nostr event JSON: %w", err)
	}
	return nipIC.ParseAttestation(&ev)
}
