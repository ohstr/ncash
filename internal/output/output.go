// Package output provides ncash's dual human/JSON rendering, its error
// classification/exit-code contract (mirroring ncli's own cli/common
// conventions so an agent or script that already knows one knows both),
// and NWC-error-to-plain-language translation for human mode.
package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// PrintJSON writes v as indented JSON to stdout — the shared success-path
// result renderer every --json command uses, so stdout only ever carries
// the operation's actual result, never narration or errors (see
// EmitError, which always writes to stderr instead).
func PrintJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "ncash: failed to encode JSON output: %s\n", err)
	}
}

// EmitError prints err to stderr — never stdout, in either mode, so a
// script parsing stdout's JSON result never has to distinguish a success
// shape from a failure shape on the same stream. Under --json this is a
// {"error","code","retryable","input"?,"nwc_code"?} object; otherwise a
// plain "Error: ..." line.
func EmitError(cmd *cobra.Command, err error) {
	if err == nil {
		return
	}
	jsonMode := false
	if cmd != nil {
		jsonMode, _ = cmd.Flags().GetBool("json")
	}
	ce := AsCLIError(err)
	if jsonMode {
		payload := map[string]any{
			"error":     ce.Err.Error(),
			"code":      string(ce.Code),
			"retryable": retryableCodes[ce.Code],
		}
		if ce.Input != "" {
			payload["input"] = ce.Input
		}
		if ce.NWCCode != "" {
			payload["nwc_code"] = ce.NWCCode
		}
		enc := json.NewEncoder(os.Stderr)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", ce.Err.Error())
}

// Linef prints a human-only narration line (a progress note, a status
// update) to stdout — a no-op under --json, since a JSON consumer only
// wants the final structured result, never narration mixed into the same
// stream it's parsing as JSON.
func Linef(jsonMode bool, format string, args ...any) {
	if jsonMode {
		return
	}
	fmt.Printf(format+"\n", args...)
}
