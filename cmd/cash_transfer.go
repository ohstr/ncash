package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/ohstr/nmilat/nipcash"
	nipcashclient "github.com/ohstr/nmilat/nipcash/client"
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/credential"
	"github.com/ohstr/ncash/internal/ledger"
	"github.com/ohstr/ncash/internal/output"
)

func newCashTransferCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Send a held cash token, in full or split",
		Args:  output.NoArgs,
		RunE:  runCashTransfer,
	}
	cmd.Flags().String("token", "", "which held token to transfer (auto-picked if you only hold one)")
	cmd.Flags().String("to", "", "recipient: pubkey:<hex> | connection:<platform>:<external-id>:<ia-pubkey> | bearer-target")
	cmd.Flags().Uint64("split", 0, "split off this much (mloki) for --to, keeping the rest — 0 transfers it all")
	cmd.Flags().String("as", "", "override credential")
	return cmd
}

func runCashTransfer(cmd *cobra.Command, args []string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	toFlag, _ := cmd.Flags().GetString("to")
	splitFlag, _ := cmd.Flags().GetUint64("split")

	// Checked explicitly (not cobra's own MarkFlagRequired) so a missing
	// --to is a classified UsageError — see cash_consolidate.go's own
	// comment on --sources for why.
	if toFlag == "" {
		return output.UsageError(cmd, fmt.Errorf("--to is required"))
	}

	target, err := credential.ParseTarget(toFlag)
	if err != nil {
		return output.InvalidInputError(cmd, toFlag, err)
	}

	l, err := ledger.Load()
	if err != nil {
		return output.RuntimeError(cmd, err)
	}
	entry, err := resolveHeldToken(cmd, l)
	if err != nil {
		return err
	}
	cred, err := resolveCredential(cmd, entry)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := nipcashclient.Connect(ctx, entry.Token)
	if err != nil {
		return output.NetworkError(cmd, err)
	}
	defer client.Close()

	amount, err := resolveAmount(cmd, l, entry, client)
	if err != nil {
		return err
	}

	var splitAmount *uint64
	message := fmt.Sprintf("This will transfer token %s (%d mloki) in full to %s.", entry.ID, amount, toFlag)
	if splitFlag > 0 {
		splitAmount = &splitFlag
		remainder := amount - splitFlag
		message = fmt.Sprintf("This sends %d mloki from %s to %s, keeping %d mloki as a new token for you.", splitFlag, entry.ID, toFlag, remainder)
	}
	if !Confirm(cmd, true, message+" Continue?") {
		fmt.Println("Cancelled.")
		return nil
	}

	result, err := client.CashTransfer(ctx, nipcash.CashTransferParams{
		Credential: cred, To: target, CurrentAmount: amount, SplitAmount: splitAmount,
	})
	if err != nil {
		return classifyNWCErr(cmd, err)
	}

	_ = l.SetStatus(entry.ID, ledger.StatusTransferred)
	l.AppendHistory("transfer", fmt.Sprintf("transferred %s to %s", entry.ID, toFlag))

	var remainderEntry *ledger.Entry
	if result.RemainderWalletToken != "" {
		remainderEntry, _ = l.Add(ledger.Entry{Token: result.RemainderWalletToken, WalletPubkey: result.RemainderWalletPubkey})
		if result.RemainingAmountMillis != nil {
			remainderEntry.AmountMillis = result.RemainingAmountMillis
		}
		l.AppendHistory("transfer", fmt.Sprintf("kept remainder as %s", remainderEntry.ID))
	}
	_ = l.Save()

	if jsonMode {
		// nipcash.CashTransferResult has no JSON tags of its own (an
		// internal SDK type, not a wire DTO) — built explicitly here so
		// --json output stays snake_case like every other ncash command's,
		// instead of leaking Go field names (see cash_inspect.go's decode
		// command for the same fix).
		output.PrintJSON(map[string]any{
			"amount_millis":           result.AmountMillis,
			"identity_type":           result.IdentityType,
			"identity_value":          result.IdentityValue,
			"remaining_amount_millis": result.RemainingAmountMillis,
			"new_wallet_pubkey":       result.NewWalletPubkey,
			"new_wallet_token":        result.NewWalletToken,
			"remainder_entry":         remainderEntry,
		})
		return nil
	}
	if remainderEntry != nil {
		fmt.Printf("Sent %d mloki. Remainder: %s (%d mloki), saved to your wallet.\n", splitFlag, remainderEntry.ID, amount-splitFlag)
	} else {
		fmt.Printf("Transferred %d mloki to %s.\n", amount, toFlag)
	}
	return nil
}
