package cmd

import (
	"fmt"

	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	shardcmds "github.com/fil-forge/libforge/commands/upload/shard"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/pkg/config"
)

var shardsPerPage uint64 = 1000

var lsFlags struct {
	proofsPath string
	showShards bool
}

func init() {
	lsCmd.Flags().StringVar(&lsFlags.proofsPath, "proof", "", "Path to a UCAN proof container with proofs for this operation.")
	lsCmd.Flags().BoolVar(&lsFlags.showShards, "shards", false, "Display shard CIDs under each upload root.")
}

var lsCmd = &cobra.Command{
	Use:     "ls <space>",
	Aliases: []string{"list"},
	Short:   "List uploads in a space",
	Long: wordwrap.WrapString(
		"Lists all uploads in the given space as CIDs, one per line. With the "+
			"`--shards` flag, lists shard CIDs below each upload root CID, indented. "+
			"The space can be specified by DID or by name.",
		80),
	Example: fmt.Sprintf("  %s ls did:key:z6MksCX5PdUgHv83cmDE2DfCrR1WHG9MmZPRKSvTi8Ca297V\n  %s ls \"my space\"", rootCmd.Name(), rootCmd.Name()),
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		if lsFlags.proofsPath != "" {
			if err := cmdutil.AddProofsFromFile(cmd.Context(), c, lsFlags.proofsPath); err != nil {
				return err
			}
		}

		spaceDID, err := cmdutil.ResolveSpace(c, args[0])
		if err != nil {
			return err
		}

		var cursor *string
		for {
			listOk, err := c.UploadList(cmd.Context(), spaceDID, uploadcmds.ListArguments{Cursor: cursor})
			if err != nil {
				return err
			}

			for _, r := range listOk.Results {
				fmt.Printf("%s\n", r.Root)
				if lsFlags.showShards {
					var shardCursor *string
					for {
						shardListOk, err := c.UploadShardList(cmd.Context(), spaceDID, shardcmds.ListArguments{
							Root:   r.Root,
							Cursor: shardCursor,
							Size:   &shardsPerPage,
						})
						if err != nil {
							return fmt.Errorf("listing shards: %w", err)
						}
						for _, s := range shardListOk.Results {
							fmt.Printf("\t%s\n", s)
						}
						shardCursor = shardListOk.Cursor
						if shardCursor == nil {
							break
						}
					}
				}
			}

			if listOk.Cursor == nil {
				break
			}
			cursor = listOk.Cursor
		}
		return nil
	},
}
