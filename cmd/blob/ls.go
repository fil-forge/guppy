package blob

import (
	"fmt"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
)

const pageSize = 1000

var lsFlags struct {
	proofsPath string
	long       bool
	human      bool
	json       bool
}

func init() {
	lsCmd.Flags().StringVar(&lsFlags.proofsPath, "proof", "", "Path to archive (CAR) containing UCAN proofs for this operation.")
	lsCmd.Flags().BoolVarP(&lsFlags.long, "long", "l", false, "Display detailed information about blobs.")
	lsCmd.Flags().BoolVarP(&lsFlags.human, "human", "H", false, "Display blob sizes in human-readable format (only applicable when used with --long).")
	lsCmd.Flags().BoolVar(&lsFlags.json, "json", false, "Output as newline delimited JSON.")
}

var lsCmd = &cobra.Command{
	Use:     "ls <space>",
	Aliases: []string{"list"},
	Short:   "List blobs in a space",
	Long: wordwrap.WrapString(
		"Lists all blobs in the given space as multibase base58btc encoded strings,"+
			" one on each line. The space can be specified by DID or by name.",
		80),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO(forrest): this command relies on client APIs removed/changed in the
		// ucantone upgrade — client.WithAdditionalProofs, c.SpaceBlobList, and
		// go-ucanto proof handling. Porting needs the new blob-list/proof flow —
		// confirm intent with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("blob ls is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}
