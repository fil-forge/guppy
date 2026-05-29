package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/libforge/identity"
)

type whoamiResult struct {
	DID string `json:"did"`
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print information about the local agent",
	Long:  "Prints information about the local agent.",

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		pem, err := os.ReadFile(cfg.Identity.KeyFile)
		if err != nil {
			return err
		}
		id, err := identity.DecodeEd25519SignerFromPEM(pem)
		if err != nil {
			return err
		}
		return output.Emit(cmd, whoamiResult{DID: id.DID().String()}, func(w io.Writer) {
			fmt.Fprintln(w, id.DID())
		})
	},
}
