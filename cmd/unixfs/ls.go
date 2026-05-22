package unixfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"text/tabwriter"

	contentcmds "github.com/fil-forge/libforge/commands/content"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/ipfs/go-cid"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/pkg/client/dagservice"
	"github.com/fil-forge/guppy/pkg/client/locator"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/guppy/pkg/dagfs"
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
		ctx := cmd.Context()

		spaceDID, err := did.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid space DID: %w", err)
		}

		parts := strings.SplitN(args[1], "/", 2)
		rootCid, err := cid.Decode(parts[0])
		if err != nil {
			return fmt.Errorf("invalid root CID: %w", err)
		}
		subPath := "."
		if len(parts) > 1 && parts[1] != "" {
			subPath = parts[1]
		}

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}

		c := cmdutil.MustGetClient(cfg)
		indexer, indexerID := cmdutil.MustGetIndexClient(cfg.Network)

		loc := locator.NewIndexLocator(indexer, func(spaces []did.DID) (ucan.Delegation, error) {
			return contentcmds.Retrieve.Delegate(c.Issuer(), indexerID, spaceDID)
		})

		dagSvc := dagservice.NewDAGService(loc, c, []did.DID{spaceDID})
		dfs := dagfs.New(ctx, dagSvc, rootCid)

		f, err := dfs.Open(subPath)
		if err != nil {
			return fmt.Errorf("opening path %s: %w", subPath, err)
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		printEntry := func(info fs.FileInfo) {
			if lsFlags.long {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", info.Mode(), info.Size(), info.ModTime().Format("Jan 02 15:04"), info.Name())
			} else {
				fmt.Println(info.Name())
			}
		}

		if !stat.IsDir() {
			printEntry(stat)
			return w.Flush()
		}

		readDirFile, ok := f.(fs.ReadDirFile)
		if !ok {
			return fmt.Errorf("directory does not support reading entries")
		}

		for {
			entries, err := readDirFile.ReadDir(20)
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("reading directory: %w", err)
			}
			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					continue
				}
				printEntry(info)
			}
			w.Flush()
		}

		return nil
	},
}

func init() {
	lsCmd.Flags().BoolVarP(&lsFlags.long, "long", "l", false, "Use long listing format")
	Cmd.AddCommand(lsCmd)
}
