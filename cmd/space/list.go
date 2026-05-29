package space

import (
	"fmt"
	"io"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/config"
)

type spaceItem struct {
	ID    string   `json:"id"`
	Names []string `json:"names,omitempty"`
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all spaces",
	Long: wordwrap.WrapString(
		"Lists all Storacha spaces stored in the local store.",
		80),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		spaces, err := c.Spaces(cmd.Context())
		if err != nil {
			return fmt.Errorf("retrieving spaces: %w", err)
		}

		items := make([]spaceItem, 0, len(spaces))
		for _, space := range spaces {
			items = append(items, spaceItem{ID: space.DID().String(), Names: space.Names()})
		}

		return output.Emit(cmd, items, func(w io.Writer) {
			for i, item := range items {
				if i == 0 {
					fmt.Fprintf(w, "%-60s %s\n", "SPACE", "NAME")
				}
				if len(item.Names) > 0 {
					quoted := make([]string, len(item.Names))
					for j, n := range item.Names {
						quoted[j] = fmt.Sprintf("%q", n)
					}
					fmt.Fprintf(w, "%-60s %s\n", item.ID, strings.Join(quoted, ", "))
				} else {
					fmt.Fprintln(w, item.ID)
				}
			}
		})
	},
}
