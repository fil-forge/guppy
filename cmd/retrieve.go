package cmd

import (
	"fmt"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
)

var retrieveCmd = &cobra.Command{
	Use:     "retrieve <space> <content-path> <output-path>",
	Aliases: []string{"get"},
	Short:   "Get a file or directory by its CID",
	Long: wordwrap.WrapString(
		"Retrieves a file or directory from a space. The specified file or "+
			"directory will be written to <output-path>. The space can be specified "+
			"by DID or by name. <content-path> can take several forms:\n\n"+
			"* /ipfs/<cid>[/<subpath>]\n"+
			"* ipfs://<cid>[/<subpath>]\n"+
			"* <cid>[/<subpath>]",
		80),
	Args: cobra.ExactArgs(3),

	RunE: func(cmd *cobra.Command, args []string) (retErr error) {
		// TODO(forrest): this command builds authorized-retrieval delegations with
		// go-ucanto (delegation.Delegate/FromDelegation, validator.NewProofPruner)
		// and the removed client.Proofs query. The client was upgraded to
		// ucantone/libforge with a different delegation/proof model. Porting needs
		// decisions on the new APIs — confirm intent with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("retrieve is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}
