// Package dial sniffs what kind of string a user pasted into `receive`,
// `join --hub`, or `connect add` — a plain NWC URI, a cash token, or one
// of the two Hub bech32 formats — without a network call
// (ncash-plan.md's "receive auto-detects what you pasted").
package dial

import (
	"strings"

	"github.com/flokiorg/go-flokicoin/chainutil/bech32"
)

// Kind identifies what Sniff found.
type Kind int

const (
	KindUnknown Kind = iota
	// KindNWCURI: a plain nostr+walletconnect:// URI.
	KindNWCURI
	// KindCashToken: a cash-token-family bech32 string (lokicash1...,
	// satscash1..., or any other HRP nipcash.Decode would accept) —
	// anything that isn't specifically a Hub connection.
	KindCashToken
	// KindCircleHub: a circlehub1... string — the one Hub kind ncash
	// actually acts on (feeds `join --hub`).
	KindCircleHub
	// KindCashHub: a cashhub1... string — recognized so a mis-paste can
	// fail with a specific error, never acted on (ncash has no mint
	// capability).
	KindCashHub
)

// hub HRPs are matched exactly; anything else that decodes as valid
// bech32 is treated as an attempted cash token — the only other bech32
// family in this ecosystem (nipcash.Decode itself accepts any HRP, so
// ncash doesn't need to enumerate lokicash/satscash/... here).
const (
	hrpCircleHub = "circlehub"
	hrpCashHub   = "cashhub"
)

// Sniff classifies s without a network call: a scheme check for the
// plain-URI case, a bare bech32 decode (HRP only, no TLV parsing) for
// everything else.
func Sniff(s string) Kind {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "nostr+walletconnect://") {
		return KindNWCURI
	}
	// DecodeNoLimit, not Decode: a cash token routinely exceeds BIP-173's
	// 90-character limit (NIP-CASH §Wire Format explicitly says
	// implementations MUST NOT enforce it) — Decode would misclassify a
	// long, perfectly valid token as KindUnknown.
	hrp, _, err := bech32.DecodeNoLimit(s)
	if err != nil {
		return KindUnknown
	}
	switch hrp {
	case hrpCircleHub:
		return KindCircleHub
	case hrpCashHub:
		return KindCashHub
	default:
		return KindCashToken
	}
}
