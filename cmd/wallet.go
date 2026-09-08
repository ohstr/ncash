package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ohstr/nmilat/nip19"
	relayclient "github.com/ohstr/nmilat/relay/client"
	"github.com/ohstr/nmilat/utils"
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/config"
	"github.com/ohstr/ncash/internal/identity"
	"github.com/ohstr/ncash/internal/ledger"
	"github.com/ohstr/ncash/internal/output"
)

func newWalletCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "Manage your identity, wallets, and their history",
	}
	cmd.AddCommand(
		newWalletInitCmd(),
		newWalletShowCmd(),
		newWalletHistoryCmd(),
		&cobra.Command{Use: "use <name>", Short: "Set the default wallet", Args: output.ExactArgs(1), RunE: runWalletUse},
		newWalletGetInfoCmd(), newWalletBalanceCmd(), newWalletBudgetCmd(),
		newWalletInvoiceCmd(), newWalletPayCmd(), newWalletListTxCmd(), newWalletSignMessageCmd(),
	)
	return cmd
}

// identityNpub returns the local identity's npub, regardless of source.
func identityNpub() (string, error) {
	s, err := identity.Load()
	if err != nil {
		return "", err
	}
	if s.Source == identity.SourceNcliVault {
		return s.Npub, nil
	}
	pubHex, err := utils.GetPublicKey(s.PrivHex)
	if err != nil {
		return "", err
	}
	return nip19.EncodePublicKey(pubHex)
}

func newWalletShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show your identity and wallet/ledger summary",
		Args:  output.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")

			idStored, err := identity.Load()
			if err != nil {
				return output.NotFoundError(cmd, "", err)
			}
			npub, err := identityNpub()
			if err != nil {
				return output.RuntimeError(cmd, err)
			}
			source := string(idStored.Source)
			if idStored.Source == identity.SourceNcliVault {
				source = fmt.Sprintf("ncli-vault:%s", idStored.Label)
			}

			s, err := config.Load()
			if err != nil {
				return output.RuntimeError(cmd, err)
			}
			l, err := ledger.Load()
			if err != nil {
				return output.RuntimeError(cmd, err)
			}

			if jsonMode {
				output.PrintJSON(map[string]any{
					"npub":            npub,
					"identity_source": source,
					"wallets":         s.Connections,
					"default_wallet":  s.Default,
					"held_tokens":     l.Held(),
				})
				return nil
			}

			fmt.Printf("Identity: %s (%s)\n", npub, source)
			fmt.Println()
			if s.IsEmpty() {
				fmt.Println("No wallets registered yet. Run `ncash join --hub ...` or `ncash connect add`.")
			} else {
				fmt.Println("Wallets:")
				for _, c := range s.Connections {
					marker := ""
					if c.Name == s.Default {
						marker = " [default]"
					}
					fmt.Printf("  %s%s\n", c.Name, marker)
				}
			}
			held := l.Held()
			fmt.Printf("\nHeld cash tokens: %d\n", len(held))
			for _, e := range held {
				amount := "unknown amount"
				if e.AmountMillis != nil {
					amount = fmt.Sprintf("%d mloki", *e.AmountMillis)
				}
				verified := ""
				if !e.Verified {
					verified = " (unverified)"
				}
				fmt.Printf("  %s: %s%s\n", e.ID, amount, verified)
			}
			return nil
		},
	}
}

func newWalletHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show your local action history",
		Args:  output.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")
			l, err := ledger.Load()
			if err != nil {
				return output.RuntimeError(cmd, err)
			}
			if jsonMode {
				output.PrintJSON(map[string]any{"history": l.History})
				return nil
			}
			if len(l.History) == 0 {
				fmt.Println("No local action history yet.")
				return nil
			}
			for _, h := range l.History {
				fmt.Printf("%s  %-12s %s\n", h.At, h.Action, h.Detail)
			}
			return nil
		},
	}
}

// dialForCommand resolves and dials the connection this command should
// act on: -c/--connection or the default wallet. Returns a *CLIError via
// output.NotFoundError when nothing is configured.
func dialForCommand(cmd *cobra.Command) (*nwcHandle, error) {
	value, ok, err := ResolveConnectionValue(cmd)
	if err != nil {
		return nil, output.RuntimeError(cmd, err)
	}
	if !ok {
		return nil, output.NotFoundError(cmd, "", errors.New(noWalletConfiguredMsg))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	client, err := DialGeneric(ctx, value)
	if err != nil {
		cancel()
		return nil, output.NetworkError(cmd, err)
	}
	return &nwcHandle{client: client, ctx: ctx, cancel: cancel}, nil
}

type nwcHandle struct {
	client *relayclient.NWCClient
	ctx    context.Context
	cancel context.CancelFunc
}

func (h *nwcHandle) Close() {
	h.cancel()
	h.client.Close()
}

func newWalletGetInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get-info",
		Short: "Show the connected wallet's capabilities",
		Args:  output.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")
			value, ok, err := ResolveConnectionValue(cmd)
			if err != nil {
				return output.RuntimeError(cmd, err)
			}
			if !ok {
				return output.NotFoundError(cmd, "", errors.New(noWalletConfiguredMsg))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			client, err := DialGeneric(ctx, value)
			if err != nil {
				return output.NetworkError(cmd, err)
			}
			defer client.Close()
			info, err := client.GetInfo(ctx)
			if err != nil {
				return classifyNWCErr(cmd, err)
			}
			if jsonMode {
				output.PrintJSON(info)
				return nil
			}
			fmt.Printf("alias:   %s\n", info.Alias)
			fmt.Printf("network: %s\n", info.Network)
			fmt.Printf("methods: %v\n", info.Methods)
			return nil
		},
	}
}

func newWalletBalanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Show your unified balance",
		Long: `Sums every locally-known wallet's real NWC balance plus every unredeemed
held cash token's value into one figure — the way a real wallet app shows
"your balance," not a protocol inventory. Use --breakdown for the itemized
per-wallet/per-token detail.`,
		Args: output.NoArgs,
		RunE: runWalletBalance,
	}
	cmd.Flags().BoolP("breakdown", "v", false, "show the itemized per-wallet/per-token detail")
	cmd.Flags().String("from", "", "show the balance of only this wallet or held token")
	return cmd
}
