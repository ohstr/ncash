// Command decode-npub is a tiny dev-only helper for bin/run.sh's
// prepare_r2_circle_join step: decode an npub1... string to its 32-byte
// hex pubkey, so the harness (on the host, not inside the agent
// container) can authorize the agent's own freshly generated identity
// under an ephemeral circle_hub's allowlist before that round proceeds.
// Not part of ncash itself — run with `go run`, never built/shipped.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/flokiorg/go-flokicoin/chainutil/bech32"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: decode-npub <npub1...>")
		os.Exit(2)
	}
	hrp, data, err := bech32.DecodeToBase256(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode npub: %v\n", err)
		os.Exit(1)
	}
	if hrp != "npub" {
		fmt.Fprintf(os.Stderr, "not an npub (hrp=%q)\n", hrp)
		os.Exit(1)
	}
	fmt.Println(hex.EncodeToString(data))
}
