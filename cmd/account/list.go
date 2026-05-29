package account

import (
	"fmt"
	"io"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/config"
)

type accountItem struct {
	ID string `json:"id"`
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List logged in accounts",
	Long: wordwrap.WrapString(
		"Lists all Storacha accounts currently logged in.",
		80),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		accounts, err := c.Accounts(cmd.Context())
		if err != nil {
			return err
		}

		items := make([]accountItem, 0, len(accounts))
		for _, account := range accounts {
			items = append(items, accountItem{ID: account.String()})
		}

		return output.Emit(cmd, items, func(w io.Writer) {
			for _, item := range items {
				fmt.Fprintln(w, item.ID)
			}
		})
	},
}
