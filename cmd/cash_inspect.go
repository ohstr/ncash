package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/ohstr/nmilat/nipcash"
	nipcashclient "github.com/ohstr/nmilat/nipcash/client"
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/ledger"
	"github.com/ohstr/ncash/internal/output"
)

func newCashListRecipientsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-recipients",
		Short: "Check your allocation and co-recipients of a held token",
		Long: `Recipients of the same mint_cash batch share one wallet connection —
this is how a receiver checks their own allocation and co-recipients
within it. It's the exact call "ncash receive --verify" makes internally.`,
		Args: output.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")
			l, err := ledger.Load()
			if err != nil {
				return output.RuntimeError(cmd, err)
			}
			entry, err := resolveHeldToken(cmd, l)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			client, err := nipcashclient.Connect(ctx, entry.Token)
			if err != nil {
				return output.NetworkError(cmd, err)
			}
			defer client.Close()
			result, err := client.ListRecipients(ctx)
			if err != nil {
				return classifyNWCErr(cmd, err)
			}
			if jsonMode {
				output.PrintJSON(result)
				return nil
			}
			for _, r := range result.Recipients {
				status := "unclaimed"
				if r.Claimed {
					status = "claimed"
					if r.ClaimedAt != nil {
						status = fmt.Sprintf("claimed %s", time.Unix(*r.ClaimedAt, 0).UTC().Format("2006-01-02"))
					}
				}
				identity := r.IdentityType
				if r.IdentityValue != "" {
					identity = fmt.Sprintf("%s:%s", r.IdentityType, r.IdentityValue)
				}
				fmt.Printf("%-40s %10d mloki   %s\n", identity, r.AmountMillis, status)
			}
			return nil
		},
	}
	cmd.Flags().String("token", "", "which held token (auto-picked if you only hold one)")
	return cmd
}

func newCashDecodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "decode <token>",
		Short: "Inspect a cash token locally, without holding it",
		Args:  output.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")
			tok, err := nipcash.Decode(args[0])
			if err != nil {
				return output.InvalidInputError(cmd, args[0], err)
			}
			if jsonMode {
				// nipcash.Token has no JSON tags of its own (it's an
				// internal SDK type, not a wire DTO) — built explicitly
				// here so decode's --json output stays snake_case like
				// every other ncash command's, instead of leaking Go
				// field names.
				out := map[string]any{
					"hrp":               tok.HRP,
					"wallet_pubkey":     tok.WalletPubkey,
					"relays":            tok.RelayURLs,
					"identity_required": tok.IdentityRequired,
				}
				if tok.HasProvenance() {
					out["mint_signature"] = fmt.Sprintf("%x", tok.MintSignature)
					out["attested_amount_millis"] = *tok.AttestedAmountMillis
				}
				output.PrintJSON(out)
				return nil
			}
			fmt.Printf("wallet_pubkey: %s\n", tok.WalletPubkey)
			fmt.Printf("relays: %s\n", joinStrings(tok.RelayURLs))
			if tok.IdentityRequired != nil {
				fmt.Printf("identity_required: %v\n", *tok.IdentityRequired)
			}
			if tok.HasProvenance() {
				fmt.Println("mint_signature: present")
				fmt.Printf("attested_amount: %d mloki\n", *tok.AttestedAmountMillis)
			}
			return nil
		},
	}
}

func newCashVerifyProvenanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify-provenance <token>",
		Short: "Verify a token's mint-signature provenance, locally",
		Args:  output.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")
			tok, err := nipcash.Decode(args[0])
			if err != nil {
				return output.InvalidInputError(cmd, args[0], err)
			}
			minter, ok := nipcash.VerifyProvenance(tok)
			if jsonMode {
				output.PrintJSON(map[string]any{"valid": ok, "minter_pubkey": minter})
				return nil
			}
			if !ok {
				fmt.Println("No valid provenance on this token.")
				return nil
			}
			fmt.Printf("Provenance valid — minted by %s", minter)
			if tok.AttestedAmountMillis != nil {
				fmt.Printf(" (matches attested amount: %d mloki)", *tok.AttestedAmountMillis)
			}
			fmt.Println(".")
			return nil
		},
	}
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
