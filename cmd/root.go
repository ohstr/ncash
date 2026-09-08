// Package cmd wires ncash's Cobra command tree. Every command here stays
// thin — the actual logic lives in internal/ (config, identity, ledger,
// credential, output, dial) so it can be unit-tested directly; commands
// that touch the network are instead exercised by the integration and
// agent-eval suites (mirroring ncli's own cli/ vs client/ split).
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/appdir"
)

// RootCmd is ncash's top-level command.
var RootCmd = &cobra.Command{
	Use:   "ncash",
	Short: "A wallet CLI for NIP-CASH cash and NIP-CW circle wallets",
	Long: `ncash is a wallet for someone who doesn't run a Hub or node themselves:
join a circle to get a personal Lightning wallet, and receive, hold, spend,
and consolidate NIP-CASH tokens — like a normal wallet.

Run "ncash init" to get started. Every command supports --json for
scripted/agentic use; see AGENTS.md for the JSON schema and exit-code
contract.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	RootCmd.PersistentFlags().Bool("json", false, "output machine-readable JSON instead of human-readable text")
	RootCmd.PersistentFlags().StringP("connection", "c", "", "use this connection/wallet by name (or a raw connection string) instead of the default")
	RootCmd.PersistentFlags().Bool("yes", false, "skip confirmation prompts")
	RootCmd.PersistentFlags().String("config-dir", "", "override ncash's local config directory (default: OS config dir)")

	RootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if dir, _ := cmd.Flags().GetString("config-dir"); dir != "" {
			appdir.SetOverride(dir)
		}
		return nil
	}
}
