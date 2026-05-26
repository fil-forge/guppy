package cmd

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"

	contentcmds "github.com/fil-forge/libforge/commands/content"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/pkg/client/dagservice"
	"github.com/fil-forge/guppy/pkg/client/locator"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/guppy/pkg/dagfs"
)

var retrieveCmd = &cobra.Command{
	Use:     "retrieve <space> <content-path> <output-path>",
	Aliases: []string{"get"},
	Short:   "Get a file or directory by its CID",
	Long: wordwrap.WrapString(
		"Retrieves a file or directory from a space. The specified file or "+
			"directory will be written to <output-path>. The space can be specified "+
			"by DID or by name. <content-path> can take several forms:\n\n"+
			"* /ipfs/<cid>[/<subpath>]\n"+
			"* ipfs://<cid>[/<subpath>]\n"+
			"* <cid>[/<subpath>]",
		80),
	Args: cobra.ExactArgs(3),

	RunE: func(cmd *cobra.Command, args []string) (retErr error) {
		ctx := cmd.Context()
		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}

		c := cmdutil.MustGetClient(cfg)
		space, err := cmdutil.ResolveSpace(c, args[0])
		if err != nil {
			return err
		}

		pathCID, subpath, err := cmdutil.ContentPath(args[1])
		if err != nil {
			cmd.SilenceUsage = false
			return fmt.Errorf("parsing content path: %w", err)
		}
		if subpath == "" {
			subpath = "."
		}

		outputPath := args[2]

		indexer, indexerID := cmdutil.MustGetIndexClient(cfg.Network)

		ctx, span := tracer.Start(ctx, "retrieve", trace.WithAttributes(
			attribute.String("retrieval.space", space.String()),
			attribute.String("retrieval.cid", pathCID.String()),
			attribute.String("retrieval.subpath", subpath),
			attribute.String("retrieval.output_path", outputPath),
		))
		defer span.End()
		defer func() {
			if retErr != nil {
				span.RecordError(retErr)
				span.SetStatus(codes.Error, "")
			}
		}()

		// Authorize the indexing service to retrieve content from the space on the
		// agent's behalf.
		loc := locator.NewIndexLocator(indexer, func(ctx context.Context, _ []did.DID) ([]ucan.Delegation, error) {
			dlg, err := contentcmds.Retrieve.Delegate(c.Issuer(), indexerID, space)
			if err != nil {
				return nil, fmt.Errorf("delegating content retrieve: %w", err)
			}
			proofs, _, err := c.ProofChain(ctx, c.Issuer().DID(), contentcmds.Retrieve.Command, space)
			if err != nil {
				return nil, fmt.Errorf("retrieving proof chain: %w", err)
			}
			return append(proofs, dlg), nil
		})

		ds := dagservice.NewDAGService(loc, c, []did.DID{space})
		retrievedFs := dagfs.New(ctx, ds, pathCID)

		file, err := retrievedFs.Open(subpath)
		if err != nil {
			return fmt.Errorf("opening path in retrieved filesystem: %w", err)
		}
		defer file.Close()

		// If it's a directory, copy the whole directory. If it's a file, copy the file.
		if _, ok := file.(fs.ReadDirFile); ok {
			span.SetAttributes(attribute.Bool("retrieval.directory", true))
			pathedFs, err := fs.Sub(retrievedFs, subpath)
			if err != nil {
				return fmt.Errorf("sub filesystem: %w", err)
			}
			if err := os.CopyFS(outputPath, pathedFs); err != nil {
				return fmt.Errorf("copying retrieved filesystem: %w", err)
			}
		} else {
			span.SetAttributes(attribute.Bool("retrieval.directory", false))
			outFile, err := os.Create(outputPath)
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			defer outFile.Close()
			if _, err := io.Copy(outFile, file); err != nil {
				return fmt.Errorf("writing to output file: %w", err)
			}
		}

		return nil
	},
}
