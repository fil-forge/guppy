package delegation

import (
	"fmt"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
)

var createFlags struct {
	can        []string
	expiration int
	output     string
}

func init() {
	createCmd.Flags().StringArrayVarP(&createFlags.can, "can", "c", nil, "One or more abilities to delegate.")
	createCmd.Flags().IntVarP(&createFlags.expiration, "expiration", "e", 0, "Unix timestamp when the delegation is no longer valid. Zero indicates no expiration.")
	createCmd.Flags().StringVarP(&createFlags.output, "output", "o", "", "Path to write the delegation CAR file to. If not specified, outputs to stdout.")
}

var createCmd = &cobra.Command{
	Use:   "create <space> <audience-did>",
	Short: "Delegate capabilities for a space to others.",
	Long: wordwrap.WrapString(
		"Output a CAR encoded UCAN that delegates capabilities for a space to the audience. "+
			"The space can be specified by DID or by name.",
		80),
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO(forrest): this command builds delegations with go-ucanto
		// (delegation.Delegate, FromDelegation, the removed client.Proofs query).
		// The client was upgraded to ucantone/libforge, which has a different
		// delegation-building and proof model. Porting it requires decisions on the
		// new APIs — confirm intent with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("delegation create is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}
