package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ohstr/nmilat/nipcash"
	nipcashclient "github.com/ohstr/nmilat/nipcash/client"
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/credential"
	"github.com/ohstr/ncash/internal/ledger"
	"github.com/ohstr/ncash/internal/output"
)

func newCashConsolidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "consolidate",
		Short: "Merge several held cash tokens into one",
		Long: `--sources takes bare local ledger IDs (amount and credential are already
known per entry) — the verbose <token>:<amount>:<credential> form is only
needed for a source that isn't in your local ledger (e.g. consolidating on
someone else's behalf via a captured proof), and only supports pubkey/
bearer credentials (a connection-key credential's own colons make it
ambiguous in this shorthand — use a locally-held entry for that case).`,
		Args: output.NoArgs,
		RunE: runCashConsolidate,
	}
	cmd.Flags().String("sources", "", "comma-separated: local ledger IDs, or <token>:<amount>:<credential>")
	cmd.Flags().String("to", "", "defaults to your own identity")
	return cmd
}

func runCashConsolidate(cmd *cobra.Command, args []string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	sourcesFlag, _ := cmd.Flags().GetString("sources")
	toFlag, _ := cmd.Flags().GetString("to")

	// Checked explicitly (not cobra's own MarkFlagRequired) so a missing
	// --sources is a classified UsageError like every other argument
	// mistake, instead of bypassing ncash's error contract entirely — see
	// output.ExactArgs's own doc comment for why cobra's built-in
	// validators are avoided throughout this command tree.
	if sourcesFlag == "" {
		return output.UsageError(cmd, fmt.Errorf("--sources is required"))
	}

	l, err := ledger.Load()
	if err != nil {
		return output.RuntimeError(cmd, err)
	}

	var sources []nipcash.Source
	var total uint64
	var localIDs []string
	for _, item := range strings.Split(sourcesFlag, ",") {
		item = strings.TrimSpace(item)
		if e, ok := l.Find(item); ok {
			cred, err := resolveCredential(cmd, e)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			amount, err := func() (uint64, error) {
				defer cancel()
				client, err := nipcashclient.Connect(ctx, e.Token)
				if err != nil {
					return 0, output.NetworkError(cmd, err)
				}
				defer client.Close()
				return resolveAmount(cmd, l, e, client)
			}()
			if err != nil {
				return err
			}
			sources = append(sources, nipcash.Source{WalletPubkey: e.WalletPubkey, Amount: amount, Credential: cred})
			total += amount
			localIDs = append(localIDs, e.ID)
			continue
		}

		parts := strings.SplitN(item, ":", 3)
		if len(parts) != 3 {
			return output.InvalidInputError(cmd, item, fmt.Errorf("not a held token ID, and not a valid <token>:<amount>:<credential> entry"))
		}
		amount, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return output.InvalidInputError(cmd, item, fmt.Errorf("invalid amount %q", parts[1]))
		}
		cred, err := credential.ParseCash(parts[2])
		if err != nil {
			return output.InvalidInputError(cmd, output.RedactSecretInput(item), err)
		}
		tok, err := decodeCashTokenForConsolidate(parts[0])
		if err != nil {
			return output.InvalidInputError(cmd, parts[0], err)
		}
		sources = append(sources, nipcash.Source{WalletPubkey: tok, Amount: amount, Credential: cred})
		total += amount
	}
	if len(sources) < 2 {
		return output.UsageError(cmd, fmt.Errorf("consolidate needs at least 2 sources, got %d", len(sources)))
	}

	var target nipcash.Target
	if toFlag != "" {
		t, err := credential.ParseTarget(toFlag)
		if err != nil {
			return output.InvalidInputError(cmd, toFlag, err)
		}
		target = t
	} else {
		myPub, err := localPubKeyHex(cmd)
		if err != nil {
			return output.RuntimeError(cmd, err)
		}
		target = nipcash.Pubkey(myPub)
	}

	if !Confirm(cmd, true, fmt.Sprintf("This will consolidate %d tokens (%d mloki total) into one%s. Continue?",
		len(sources), total, ifTargetIsSelf(toFlag))) {
		fmt.Println("Cancelled.")
		return nil
	}

	// Any connected source's client works to place the call — sources are
	// authorized per-source by their own proof, not by the calling
	// connection (see NIP-CASH §Consolidating Tokens).
	firstEntry, _ := l.Find(localIDs[0])
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := nipcashclient.Connect(ctx, firstEntry.Token)
	if err != nil {
		return output.NetworkError(cmd, err)
	}
	defer client.Close()

	result, err := client.CashConsolidate(ctx, nipcash.CashConsolidateParams{Sources: sources, To: target})
	if err != nil {
		return classifyNWCErr(cmd, err)
	}

	for _, id := range localIDs {
		_ = l.SetStatus(id, ledger.StatusConsolidated)
	}
	newEntry, _ := l.Add(ledger.Entry{Token: result.NewWalletToken, WalletPubkey: result.NewWalletPubkey, AmountMillis: &result.AmountMillis})
	l.AppendHistory("consolidate", fmt.Sprintf("consolidated %v into %s", localIDs, newEntry.ID))
	_ = l.Save()

	if jsonMode {
		// new_entry (a ledger.Entry, properly snake_case-tagged) already
		// carries result's token/wallet_pubkey/amount — expires_at is the
		// only field of nipcash.CashConsolidateResult (no JSON tags of its
		// own) worth surfacing separately (see cash_transfer.go's own fix
		// for the same underlying issue).
		output.PrintJSON(map[string]any{"new_entry": newEntry, "expires_at": result.ExpiresAt})
		return nil
	}
	fmt.Printf("Consolidated → %s (%d mloki), saved to your wallet.\n", newEntry.ID, result.AmountMillis)
	return nil
}

func ifTargetIsSelf(to string) string {
	if to == "" {
		return ", sent to your own identity"
	}
	return ""
}

// decodeCashTokenForConsolidate decodes a bare token string (the verbose
// non-ledger source form) down to just its wallet pubkey.
func decodeCashTokenForConsolidate(token string) (string, error) {
	tok, err := nipcash.Decode(token)
	if err != nil {
		return "", err
	}
	return tok.WalletPubkey, nil
}
