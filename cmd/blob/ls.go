package blob

import (
	"github.com/dustin/go-humanize"
	blobcmds "github.com/fil-forge/libforge/commands/blob"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/pkg/config"
)

const pageSize = 1000

var lsFlags struct {
	proofsPath string
	long       bool
	human      bool
	json       bool
}

func init() {
	lsCmd.Flags().StringVar(&lsFlags.proofsPath, "proof", "", "Path to a UCAN proof container with proofs for this operation.")
	lsCmd.Flags().BoolVarP(&lsFlags.long, "long", "l", false, "Display detailed information about blobs.")
	lsCmd.Flags().BoolVar(&lsFlags.json, "json", false, "Output as newline delimited JSON.")
	lsCmd.Flags().BoolVarP(&lsFlags.human, "human", "H", false, "Display blob sizes in human-readable format (only applicable with --long).")
}

var lsCmd = &cobra.Command{
	Use:     "ls <space>",
	Aliases: []string{"list"},
	Short:   "List blobs in a space",
	Long: wordwrap.WrapString(
		"Lists all blobs in the given space as multibase base58btc encoded digests, "+
			"one per line. The space can be specified by DID or by name.",
		80),
	Args: cobra.ExactArgs(1),
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
		size := uint64(pageSize)
		for {
			listOK, err := c.BlobList(cmd.Context(), spaceDID, blobcmds.ListArguments{Cursor: cursor, Size: &size})
			if err != nil {
				return err
			}

			for _, r := range listOK.Results {
				switch {
				case lsFlags.json:
					err := r.MarshalDagJSON(cmd.OutOrStdout())
					if err != nil {
						return err
					}
					cmd.Println("")
				case lsFlags.long && lsFlags.human:
					cmd.Printf("%s\t%s\n", digestutil.Format(r.Blob.Digest), humanize.IBytes(r.Blob.Size))
				case lsFlags.long:
					cmd.Printf("%s\t%d\n", digestutil.Format(r.Blob.Digest), r.Blob.Size)
				default:
					cmd.Println(digestutil.Format(r.Blob.Digest))
				}
			}

			if listOK.Cursor == nil {
				break
			}
			cursor = listOK.Cursor
		}
		return nil
	},
}
