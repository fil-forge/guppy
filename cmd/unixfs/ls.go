package unixfs

import (
	"fmt"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/spf13/cobra"
)

var lsFlags struct {
	long bool
}

var lsCmd = &cobra.Command{
	Use:   "ls <space-did> <cid-path>",
	Short: "List directory contents",
	Long:  "Lists files and directories in a UnixFS tree. Supports shallow listing and incremental output.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO(forrest): this command builds a consumer/get retrieval delegation
		// with go-ucanto (consumer.Get.Delegate, delegation.FromDelegation) and the
		// removed client.Proofs query. The client was upgraded to ucantone/libforge
		// with a different delegation/proof model. Porting needs decisions on the new
		// APIs — confirm intent with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("unixfs ls is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}

func init() {
	lsCmd.Flags().BoolVarP(&lsFlags.long, "long", "l", false, "Use long listing format")
	Cmd.AddCommand(lsCmd)
}
