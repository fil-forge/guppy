package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/build"
)

type versionResult struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
	BuiltBy string `json:"built_by"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of guppy",
	Long:  `Print the version of guppy including the git revision.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return output.Emit(cmd, versionResult{
			Version: build.Version,
			Commit:  build.Commit,
			BuiltAt: build.Date,
			BuiltBy: build.BuiltBy,
		}, func(w io.Writer) {
			fmt.Fprintf(w, "version: %s\n", build.Version)
			fmt.Fprintf(w, "commit: %s\n", build.Commit)
			fmt.Fprintf(w, "built at: %s\n", build.Date)
			fmt.Fprintf(w, "built by: %s\n", build.BuiltBy)
		})
	},
}
