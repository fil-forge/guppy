package cmd

import (
	"fmt"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
)

var shardsPerPage uint64 = 1000

var lsFlags struct {
	proofsPath string
	showShards bool
}

func init() {
	lsCmd.Flags().StringVar(&lsFlags.proofsPath, "proof", "", "Path to archive (CAR) containing UCAN proofs for this operation.")
	lsCmd.Flags().BoolVar(&lsFlags.showShards, "shards", false, "Display shard CIDs under each upload root.")
}

var lsCmd = &cobra.Command{
	Use:     "ls <space>",
	Aliases: []string{"list"},
	Short:   "List uploads in a space",
	Long: wordwrap.WrapString(
		"Lists all uploads in the given space as CIDs, one on each line. With "+
			"`--shards` flag, lists shard CIDs below each upload root CID, indented. "+
			"The space can be specified by DID or by name.",
		80),
	Example: fmt.Sprintf("  %s ls did:key:z6MksCX5PdUgHv83cmDE2DfCrR1WHG9MmZPRKSvTi8Ca297V\n  %s ls \"my space\"", rootCmd.Name(), rootCmd.Name()),
	Args:    cobra.ExactArgs(1),

	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO(forrest): listing relies on client.WithAdditionalProofs (removed in
		// the ucantone upgrade — proofs now come from the token store) and the
		// go-libstoracha ListCaveats types, which became libforge ListArguments.
		// Porting needs the new proof-supply path and argument types — confirm
		// intent with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("ls is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}
