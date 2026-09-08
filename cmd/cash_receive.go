package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ohstr/nmilat/nipcash"
	nipcashclient "github.com/ohstr/nmilat/nipcash/client"
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/dial"
	"github.com/ohstr/ncash/internal/ledger"
	"github.com/ohstr/ncash/internal/output"
)

func newCashReceiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "receive <token>",
		Short: `"Cash-in" a token: decode it and add it to your local wallet`,
		Long: `Decodes a cash token locally and records it in your wallet immediately,
marked unverified — no network call, so it feels instant. Pass --verify to
cross-check it against the Hub.

For a bearer-mode token, also pass --secret <bearer_secret> if you have
it: the token's own connection alone is NEVER enough to redeem/transfer
a bearer slice (it only lets you dial the wallet) — the actual spending
credential is a separate value the Hub operator hands out alongside the
token, once, at mint time. Without it now, you'll need --as
bearer:<secret> when you later redeem or transfer this entry.`,
		Args: output.ExactArgs(1),
		RunE: runCashReceive,
	}
	cmd.Flags().Bool("verify", false, "cross-check against the Hub via list_recipients")
	cmd.Flags().String("secret", "", "the bearer_secret handed out alongside a bearer-mode token (bearer tokens only)")
	return cmd
}

func runCashReceive(cmd *cobra.Command, args []string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	verify, _ := cmd.Flags().GetBool("verify")
	secretFlag, _ := cmd.Flags().GetString("secret")
	input := args[0]

	switch dial.Sniff(input) {
	case dial.KindCircleHub:
		return output.InvalidInputError(cmd, input, fmt.Errorf("that's a Circle Hub connection — use `ncash join --hub <connection>` to join it"))
	case dial.KindCashHub:
		return output.InvalidInputError(cmd, input, fmt.Errorf("that's a Cash Hub connection (for minting cash) — ncash can't mint, this needs the Hub operator's own tooling"))
	case dial.KindNWCURI:
		return output.InvalidInputError(cmd, input, fmt.Errorf("that looks like a wallet connection, not a cash token — use `ncash connect add <name> <uri>` instead"))
	case dial.KindUnknown:
		return output.InvalidInputError(cmd, input, fmt.Errorf("doesn't look like a valid cash token"))
	}

	tok, err := nipcash.Decode(input)
	if err != nil {
		return output.InvalidInputError(cmd, input, err)
	}

	isBearer := tok.IdentityRequired != nil && !*tok.IdentityRequired
	if secretFlag != "" && !isBearer {
		return output.UsageError(cmd, fmt.Errorf("--secret only applies to a bearer-mode token — this one requires an identity proof, not a bearer secret"))
	}

	l, err := ledger.Load()
	if err != nil {
		return output.RuntimeError(cmd, err)
	}

	entry := ledger.Entry{
		Token:            input,
		WalletPubkey:     tok.WalletPubkey,
		Secret:           tok.Secret,
		BearerSecret:     secretFlag,
		RelayURLs:        tok.RelayURLs,
		IdentityRequired: tok.IdentityRequired,
	}
	if tok.HasProvenance() {
		entry.AmountMillis = tok.AttestedAmountMillis
	}

	added, err := l.Add(entry)
	if err != nil {
		if errors.Is(err, ledger.ErrAlreadyHeld) {
			return output.ConflictError(cmd, input, err)
		}
		return output.RuntimeError(cmd, err)
	}
	l.AppendHistory("receive", fmt.Sprintf("received token %s", added.ID))

	var verifyWarning string
	if verify {
		if err := verifyReceivedToken(cmd, l, added); err != nil {
			verifyWarning = err.Error()
		}
	}

	if err := l.Save(); err != nil {
		return output.RuntimeError(cmd, err)
	}

	if jsonMode {
		output.PrintJSON(map[string]any{"entry": added, "verify_warning": verifyWarning})
		return nil
	}

	amountStr := "an unknown amount"
	if added.AmountMillis != nil {
		amountStr = fmt.Sprintf("%d mloki", *added.AmountMillis)
	}
	bearerNote := ""
	if added.IdentityRequired != nil && !*added.IdentityRequired {
		if added.BearerSecret != "" {
			bearerNote = " (bearer note — its secret is stored locally, ready to redeem/transfer)"
		} else {
			bearerNote = " (bearer note — no --secret given; you'll need --as bearer:<secret> to redeem or transfer it)"
		}
	}
	fmt.Printf("Received %s%s. Saved as %s", amountStr, bearerNote, added.ID)
	if added.Verified {
		fmt.Println(", verified.")
	} else {
		fmt.Println(", unverified.")
		if verifyWarning != "" {
			fmt.Printf("Warning: could not verify against the Hub: %s\n", verifyWarning)
		}
	}
	return nil
}

// verifyReceivedToken cross-checks a freshly received token against the
// Hub via list_recipients — the exact call `list-recipients` itself makes
// (see ncash-plan.md's "it's the exact call receive --verify makes
// internally"). Best-effort match: a bearer entry needs no identity to
// compare against; a pubkey entry is matched against the local identity's
// own pubkey (the only identity a "received for me" token would be bound
// to). A connection-key entry can't be matched this way — it isn't
// something Decode can tell us, so it's left unverified either way.
func verifyReceivedToken(cmd *cobra.Command, l *ledger.Ledger, e *ledger.Entry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := nipcashclient.Connect(ctx, e.Token)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ListRecipients(ctx)
	if err != nil {
		return err
	}

	isBearer := e.IdentityRequired != nil && !*e.IdentityRequired
	var myPubHex string
	if !isBearer {
		myPubHex, _ = localPubKeyHex(cmd) // best-effort; empty means no pubkey match attempted
	}

	for _, r := range result.Recipients {
		matched := (isBearer && r.IdentityType == "bearer") ||
			(!isBearer && r.IdentityType == "pubkey" && myPubHex != "" && r.IdentityValue == myPubHex)
		if matched {
			amount := r.AmountMillis
			e.AmountMillis = &amount
			return l.SetVerified(e.ID, true)
		}
	}
	return fmt.Errorf("no matching recipient found on the Hub")
}
