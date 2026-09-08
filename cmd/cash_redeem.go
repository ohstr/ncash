package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/ohstr/nmilat/nip47"
	"github.com/ohstr/nmilat/nipcash"
	nipcashclient "github.com/ohstr/nmilat/nipcash/client"
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/config"
	"github.com/ohstr/ncash/internal/credential"
	"github.com/ohstr/ncash/internal/ledger"
	"github.com/ohstr/ncash/internal/output"
)

func newCashRedeemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "redeem",
		Short: "Redeem a held cash token into a Lightning wallet",
		Long: `cash_redeem always needs a destination invoice on the wire — ncash
generates one for you: --token picks the source (auto-picked when you only
hold one), --to picks the destination wallet (defaults to your default
wallet). --invoice bypasses both, redeeming straight into an invoice from
any other wallet app you already have — no ncash-registered wallet needed.`,
		Args: output.NoArgs,
		RunE: runCashRedeem,
	}
	cmd.Flags().String("token", "", "which held token to redeem (auto-picked if you only hold one)")
	cmd.Flags().String("to", "", "which wallet to redeem into (defaults to your default wallet)")
	cmd.Flags().String("invoice", "", "redeem straight into this external invoice")
	cmd.Flags().String("as", "", "override credential (pubkey:<priv> | connection-key:... | bearer:<secret>)")
	return cmd
}

func runCashRedeem(cmd *cobra.Command, args []string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	explicitInvoice, _ := cmd.Flags().GetString("invoice")

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

	sourceClient, err := nipcashclient.Connect(ctx, entry.Token)
	if err != nil {
		return output.NetworkError(cmd, err)
	}
	defer sourceClient.Close()

	var invoice string
	var destName string
	if explicitInvoice != "" {
		invoice = explicitInvoice
		destName = "the invoice above"
	} else {
		amount, err := resolveAmount(cmd, l, entry, sourceClient)
		if err != nil {
			return err
		}
		destWalletName, destValue, err := resolveDestWallet(cmd)
		if err != nil {
			return err
		}
		destClient, err := DialGeneric(ctx, destValue)
		if err != nil {
			return output.NetworkError(cmd, err)
		}
		defer destClient.Close()
		tx, err := destClient.MakeInvoice(ctx, nip47.MakeInvoiceParams{Amount: int64(amount)})
		if err != nil {
			return classifyNWCErr(cmd, err)
		}
		invoice = tx.Invoice
		destName = destWalletName
	}

	amountNote := ""
	if entry.AmountMillis != nil {
		amountNote = fmt.Sprintf(" (%d mloki)", *entry.AmountMillis)
	}
	if !Confirm(cmd, true, fmt.Sprintf("This will redeem token %s%s into %s. Continue?", entry.ID, amountNote, destName)) {
		fmt.Println("Cancelled.")
		return nil
	}

	result, err := sourceClient.CashRedeem(ctx, nipcash.CashRedeemParams{Invoice: invoice, Credential: cred})
	if err != nil {
		return classifyNWCErr(cmd, err)
	}

	_ = l.SetStatus(entry.ID, ledger.StatusRedeemed)
	l.AppendHistory("redeem", fmt.Sprintf("redeemed %s into %s", entry.ID, destName))
	_ = l.Save()

	if jsonMode {
		output.PrintJSON(map[string]any{
			"redeemed_token": entry.ID, "to_wallet": destName,
			"fee_mloki": result.FeesPaid, "preimage": result.Preimage,
		})
		return nil
	}
	fmt.Printf("Redeemed → %s.\n", destName)
	if result.FeesPaid > 0 {
		fmt.Printf("Fee: %d mloki.\n", result.FeesPaid)
	}
	return nil
}

// resolveHeldToken picks which held token to act on: --token by ID, or
// auto-picked when exactly one is held — see ncash-plan.md's "auto-picked
// when only one held token qualifies" rule.
func resolveHeldToken(cmd *cobra.Command, l *ledger.Ledger) (*ledger.Entry, error) {
	if id, _ := cmd.Flags().GetString("token"); id != "" {
		e, ok := l.Find(id)
		if !ok {
			return nil, output.NotFoundError(cmd, id, fmt.Errorf("no held token %q", id))
		}
		return e, nil
	}
	held := l.Held()
	if len(held) == 0 {
		return nil, output.NotFoundError(cmd, "", fmt.Errorf("you have no held cash tokens — receive one first with `ncash receive <token>`"))
	}
	if len(held) > 1 {
		return nil, output.UsageError(cmd, fmt.Errorf("you hold %d cash tokens — specify which with --token <id> (see `ncash wallet show`)", len(held)))
	}
	return &held[0], nil
}

// resolveCredential resolves the credential to redeem/transfer with:
// --as overrides everything; a bearer-mode token uses its stored
// BearerSecret if `receive --secret` captured one (the token's own Secret
// field is NEVER a valid substitute — it's only the NWC connection secret,
// not the separate spending credential a bearer slice needs; see
// ledger.Entry.BearerSecret's own doc comment and NIP-CASH.md's Redemption
// Metadata section), otherwise it requires an explicit --as; a
// connection-key-bound token currently also requires an explicit --as
// (re-deriving a fresh live attestation automatically is a known gap —
// see ledger.Entry's own doc comment on why only a *reference* is
// stored); everything else defaults to the local identity.
func resolveCredential(cmd *cobra.Command, entry *ledger.Entry) (nipcash.Credential, error) {
	if as, _ := cmd.Flags().GetString("as"); as != "" {
		cred, err := credential.ParseCash(as)
		if err != nil {
			return nil, output.InvalidInputError(cmd, output.RedactSecretInput(as), err)
		}
		return cred, nil
	}
	if entry.IdentityRequired != nil && !*entry.IdentityRequired {
		if entry.BearerSecret != "" {
			return nipcash.BySecret(entry.BearerSecret), nil
		}
		return nil, output.UsageError(cmd, fmt.Errorf(
			"this bearer-mode token's spending secret wasn't captured at receive time — pass --as bearer:<secret> for %s", entry.ID))
	}
	if entry.ConnectionKeyPlatform != "" {
		return nil, output.UsageError(cmd, fmt.Errorf(
			"this token is connection-key-bound — pass --as connection-key:<privkey>,%s,%s,<attestation-file>",
			entry.ConnectionKeyPlatform, entry.ConnectionKeyExternalID))
	}
	cred, err := localCashCredential(cmd)
	if err != nil {
		return nil, output.RuntimeError(cmd, err)
	}
	return cred, nil
}

// resolveDestWallet resolves --to (by store name, or a raw value), or the
// default wallet.
func resolveDestWallet(cmd *cobra.Command) (name, value string, err error) {
	to, _ := cmd.Flags().GetString("to")
	s, loadErr := config.Load()
	if loadErr != nil {
		return "", "", output.RuntimeError(cmd, loadErr)
	}
	if to != "" {
		if c, ok := s.Find(to); ok {
			return c.Name, c.Value, nil
		}
		return to, to, nil
	}
	c, ok := s.DefaultConnection()
	if !ok {
		return "", "", output.NotFoundError(cmd, "", fmt.Errorf(noWalletConfiguredMsg+"\n  Or redeem straight into an invoice from any other wallet app: ncash redeem --invoice <bolt11>"))
	}
	return c.Name, c.Value, nil
}

// resolveAmount returns entry's known amount, or discovers it live via
// list_recipients if it isn't cached yet (an unverified token received
// without --verify) — auto-verifying on demand rather than making the
// user run `receive --verify` first just to redeem.
func resolveAmount(cmd *cobra.Command, l *ledger.Ledger, entry *ledger.Entry, sourceClient *nipcashclient.Client) (uint64, error) {
	if entry.AmountMillis != nil {
		return *entry.AmountMillis, nil
	}
	result, err := sourceClient.ListRecipients(context.Background())
	if err != nil {
		return 0, classifyNWCErr(cmd, err)
	}
	isBearer := entry.IdentityRequired != nil && !*entry.IdentityRequired
	var myPubHex string
	if !isBearer {
		myPubHex, _ = localPubKeyHex(cmd)
	}
	for _, r := range result.Recipients {
		if (isBearer && r.IdentityType == "bearer") || (!isBearer && r.IdentityType == "pubkey" && myPubHex != "" && r.IdentityValue == myPubHex) {
			entry.AmountMillis = &r.AmountMillis
			_ = l.SetVerified(entry.ID, true)
			return r.AmountMillis, nil
		}
	}
	return 0, output.NotFoundError(cmd, entry.ID, fmt.Errorf("couldn't determine this token's amount — try `ncash receive --verify` first, or check it's still valid"))
}
