package cmd

import (
	"errors"

	relayclient "github.com/ohstr/nmilat/relay/client"
	"github.com/spf13/cobra"

	"github.com/ohstr/ncash/internal/output"
)

// classifyNWCErr turns a wallet-call error into ncash's own classified
// *CLIError: a *relayclient.WalletError becomes output.NWCError (plain-
// language translation, correct exit code); anything else (a dial/network
// failure) becomes output.NetworkError.
func classifyNWCErr(cmd *cobra.Command, err error) error {
	var walletErr *relayclient.WalletError
	if errors.As(err, &walletErr) {
		return output.NWCError(cmd, walletErr)
	}
	return output.NetworkError(cmd, err)
}
