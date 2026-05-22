package proof

import (
	"fmt"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
)

var addCmd = &cobra.Command{
	Use:   "add <data-or-path>",
	Short: "Add a proof delegated to this agent.",
	Long: wordwrap.WrapString(
		"Parse or decode a proof from the given data or file path. A proof is a delegation for this agent.",
		80),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO(forrest): this command parses a go-ucanto delegation
		// (delegation.Parse/Extract) and calls the changed client.AddProofs
		// (now (ctx, ...ucantone-delegation)). The client was upgraded to
		// ucantone/libforge; porting needs the ucantone delegation/container
		// decode path — confirm intent with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("proof add is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}
