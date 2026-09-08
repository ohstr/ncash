//go:build integration

package integration

import (
	"encoding/hex"
	"fmt"

	"github.com/flokiorg/go-flokicoin/chainutil/bech32"
)

// npubToHex decodes a standard NIP-19 "npub1..." string to its 32-byte hex
// pubkey. ncash's CLI only ever prints npub (never hex) for a freshly
// generated local identity, so this suite decodes it itself to authorize
// that pubkey under an ephemeral circle_hub's allowlist.
func npubToHex(npub string) (string, error) {
	hrp, data, err := bech32.DecodeToBase256(npub)
	if err != nil {
		return "", fmt.Errorf("decode npub: %w", err)
	}
	if hrp != "npub" {
		return "", fmt.Errorf("not an npub (hrp=%q)", hrp)
	}
	return hex.EncodeToString(data), nil
}
