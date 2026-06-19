package source

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/guppy/pkg/preparation"
)

type sourceItem struct {
	SourceID string `json:"source_id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
}

var ListCmd = &cobra.Command{
	Use:     "list <space>",
	Aliases: []string{"ls"},
	Short:   "List sources added to a space",
	Long:    `Lists the sources added to a space. The space can be specified by DID or by name.`,
	Args:    cobra.ExactArgs(1),

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		repo, err := preparation.OpenRepo(ctx, cfg.Repo)
		if err != nil {
			return err
		}
		defer repo.Close()

		spaceArg := cmd.Flags().Arg(0)
		if spaceArg == "" {
			cmd.SilenceUsage = false
			return fmt.Errorf("space cannot be empty")
		}

		spaceDID, err := cmdutil.ResolveSpace(cmd.Context(), cmdutil.MustGetClient(cfg), spaceArg)
		if err != nil {
			return err
		}

		sourceIDs, err := repo.ListSpaceSources(ctx, spaceDID)
		if err != nil {
			return err
		}

		items := make([]sourceItem, 0, len(sourceIDs))
		for _, sourceID := range sourceIDs {
			source, err := repo.GetSourceByID(ctx, sourceID)
			if err != nil {
				return fmt.Errorf("failed to get source by ID %s: %w", sourceID, err)
			}
			items = append(items, sourceItem{
				SourceID: source.ID().String(),
				Name:     source.Name(),
				Path:     source.Path(),
			})
		}

		return output.Emit(cmd, items, func(w io.Writer) {
			fmt.Fprintf(w, "Sources for space %s:\n", spaceDID)
			for _, item := range items {
				if item.Name != item.Path {
					fmt.Fprintf(w, "- %s: %s\n", item.Name, item.Path)
				} else {
					fmt.Fprintf(w, "- %s\n", item.Path)
				}
			}
			if len(items) == 0 {
				fmt.Fprintf(w, "No sources found for space %s. Add a source first with:\n\n$ %s %s <path>\n\n", spaceDID, AddCmd.CommandPath(), spaceDID)
			}
		})
	},
}
