package blob

import (
	"fmt"
	"io"

	"github.com/dustin/go-humanize"
	blobcmds "github.com/fil-forge/libforge/commands/blob"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/config"
)

const pageSize = 1000

type blobItem struct {
	Digest string `json:"digest"`
	Size   uint64 `json:"size"`
}

var lsFlags struct {
	proofsPath string
	long       bool
	human      bool
}

func init() {
	lsCmd.Flags().StringVar(&lsFlags.proofsPath, "proof", "", "Path to a UCAN proof container with proofs for this operation.")
	lsCmd.Flags().BoolVarP(&lsFlags.long, "long", "l", false, "Display detailed information about blobs.")
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

		spaceDID, err := cmdutil.ResolveSpace(cmd.Context(), c, args[0])
		if err != nil {
			return err
		}

		var items []blobItem
		var cursor *string
		size := uint64(pageSize)
		for {
			listOK, err := c.BlobList(cmd.Context(), spaceDID, blobcmds.ListArguments{Cursor: cursor, Size: &size})
			if err != nil {
				return err
			}

			for _, r := range listOK.Results {
				items = append(items, blobItem{Digest: digestutil.Format(r.Blob.Digest), Size: r.Blob.Size})
			}

			if listOK.Cursor == nil {
				break
			}
			cursor = listOK.Cursor
		}

		return output.Emit(cmd, items, func(w io.Writer) {
			for _, item := range items {
				switch {
				case lsFlags.long && lsFlags.human:
					fmt.Fprintf(w, "%s\t%s\n", item.Digest, humanize.IBytes(item.Size))
				case lsFlags.long:
					fmt.Fprintf(w, "%s\t%d\n", item.Digest, item.Size)
				default:
					fmt.Fprintln(w, item.Digest)
				}
			}
		})
	},
}
