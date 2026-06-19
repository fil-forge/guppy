package space

import (
	"fmt"
	"io"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/config"
)

type spaceInfoResult struct {
	Space     string   `json:"space"`
	Providers []string `json:"providers"`
}

var infoCmd = &cobra.Command{
	Use:   "info <space>",
	Short: "Get information about a space",
	Long: wordwrap.WrapString(
		"Gets information about a space, including which providers are associated with it. "+
			"This shows the space's provisioning status. The space can be specified by DID or by name.",
		80),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		spaceDID, err := cmdutil.ResolveSpace(cmd.Context(), c, args[0])
		if err != nil {
			return err
		}

		result, err := c.SpaceInfo(cmd.Context(), spaceDID)
		if err != nil {
			return fmt.Errorf("getting space info: %w", err)
		}

		providers := make([]string, 0, len(result.Providers))
		for _, provider := range result.Providers {
			providers = append(providers, provider.String())
		}

		return output.Emit(cmd, spaceInfoResult{Space: spaceDID.String(), Providers: providers}, func(w io.Writer) {
			fmt.Fprintf(w, "Space: %s\n", spaceDID)
			if len(providers) > 0 {
				fmt.Fprintf(w, "Providers:\n")
				for _, provider := range providers {
					fmt.Fprintf(w, "  - %s\n", provider)
				}
			} else {
				fmt.Fprintf(w, "No providers (space not provisioned)\n")
			}
		})
	},
}
