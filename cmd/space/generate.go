package space

import (
	"fmt"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
)

// spaceAccess is the set of capabilities required by the agent to manage a
// space.
var spaceAccess = []string{
	"assert/*",
	"space/*",
	"blob/*",
	"index/*",
	"store/*",
	"upload/*",
	"access/*",
	"filecoin/*",
	"usage/*",
}

var generateFlags struct {
	name        string
	grantTo     string
	provisionTo string
	outputKey   bool
}

func init() {
	generateCmd.Flags().StringVar(&generateFlags.name, "name", "", "Name for the space (optional)")
	generateCmd.Flags().StringVar(&generateFlags.grantTo, "grant-to", "", "Account DID to grant space access to. Must be logged in already. (optional when exactly one account is logged in)")
	generateCmd.Flags().StringVar(&generateFlags.provisionTo, "provision-to", "", "Account DID to provision space to. Must be logged in already. (optional when exactly one account is logged in)")
	generateCmd.Flags().BoolVarP(&generateFlags.outputKey, "output-key", "k", false, "Output the space key (WARNING: sensitive data)")
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new space",
	Long: wordwrap.WrapString(
		"Generates a new Storacha space, provisions it to the logged-in account, "+
			"grants space access to the logged-in account, and stores it in the "+
			"local store.",
		80),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO(forrest): space generation provisions and grants access by building
		// delegations with go-ucanto (signer.Generate, delegation.Delegate), and
		// relies on client APIs that changed/were removed in the ucantone upgrade
		// (c.Connection, ProviderAdd/AccessDelegate did types, AddProofs signature).
		// Porting needs decisions on the new delegation/grant flow — confirm intent
		// with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("space generate is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}
