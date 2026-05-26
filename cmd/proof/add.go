package proof

import (
	"fmt"
	"os"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/ucantone/ucan/container"
)

var addCmd = &cobra.Command{
	Use:   "add <data-or-path>",
	Short: "Add a proof delegated to this agent.",
	Long: wordwrap.WrapString(
		"Decode a proof from the given data or file path and store it for this agent. "+
			"A proof is a UCAN delegation container (e.g. one produced by `guppy delegation create`).",
		80),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// The argument may be a path to a container file or the encoded container
		// data itself.
		data, err := os.ReadFile(args[0])
		if err != nil {
			data = []byte(args[0])
		}

		ct, err := container.Decode(data)
		if err != nil {
			return fmt.Errorf("decoding proof container: %w", err)
		}
		dels := ct.Delegations()
		if len(dels) == 0 {
			return fmt.Errorf("no delegations found in the proof")
		}

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		if err := c.AddProofs(cmd.Context(), dels...); err != nil {
			return fmt.Errorf("adding proofs: %w", err)
		}

		cmd.PrintErrf("Added %d proof(s).\n", len(dels))
		return nil
	},
}
