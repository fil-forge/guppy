package delegation

import (
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/command"
	"github.com/fil-forge/ucantone/ucan/container"
	"github.com/fil-forge/ucantone/ucan/delegation"
)

var createFlags struct {
	cmds       []string
	expiration int
	output     string
}

func init() {
	createCmd.Flags().StringArrayVarP(&createFlags.cmds, "cmd", "c", nil, "One or more commands to delegate, e.g. /blob/add.")
	createCmd.Flags().IntVarP(&createFlags.expiration, "expiration", "e", 0, "Unix timestamp when the delegation is no longer valid. Zero indicates no expiration.")
	createCmd.Flags().StringVar(&createFlags.output, "out", "", "Path to write the delegation container to. If not specified, outputs to stdout.")
}

var createCmd = &cobra.Command{
	Use:   "create <space> <audience-did>",
	Short: "Delegate capabilities for a space to others.",
	Long: wordwrap.WrapString(
		"Output a UCAN delegation container that delegates capabilities for a space to "+
			"the audience, bundled with the proof chain that authorizes them. The space "+
			"can be specified by DID or by name. The output can be loaded by the audience "+
			"with `guppy proof add`.",
		80),
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		aud, err := did.Parse(args[1])
		if err != nil {
			return fmt.Errorf("parsing audience DID: %w", err)
		}
		if len(createFlags.cmds) == 0 {
			return fmt.Errorf("at least one command must be specified with --cmd")
		}

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		space, err := cmdutil.ResolveSpace(cmd.Context(), c, args[0])
		if err != nil {
			return err
		}

		var opts []delegation.Option
		if createFlags.expiration > 0 {
			opts = append(opts, delegation.WithExpiration(ucan.UnixTimestamp(createFlags.expiration)))
		} else {
			opts = append(opts, delegation.WithNoExpiration())
		}

		// Collect the new delegations plus the proof chain authorizing each,
		// deduplicating by CID.
		var dels []ucan.Delegation
		for _, cmdStr := range createFlags.cmds {
			cmdName, err := command.Parse(normalizeCommand(cmdStr))
			if err != nil {
				return fmt.Errorf("parsing command %q: %w", cmdStr, err)
			}

			proofs, _, err := c.ProofChain(cmd.Context(), c.Issuer().DID(), cmdName, space)
			if err != nil {
				return fmt.Errorf("building proof chain for %q: %w", cmdStr, err)
			}
			if len(proofs) == 0 {
				return fmt.Errorf("no delegations found for command %q with space %q", cmdStr, space)
			}

			del, err := delegation.Delegate(c.Issuer(), aud, space, cmdName, opts...)
			if err != nil {
				return fmt.Errorf("creating delegation: %w", err)
			}
			dels = append(dels, del)
			dels = append(dels, proofs...)
		}

		encoded, err := container.Encode(container.Base64Gzip, container.New(container.WithDelegations(dels...)))
		if err != nil {
			return fmt.Errorf("encoding delegation container: %w", err)
		}

		if createFlags.output != "" {
			if err := os.WriteFile(createFlags.output, encoded, 0644); err != nil {
				return fmt.Errorf("writing delegation container: %w", err)
			}
		} else {
			fmt.Println(string(encoded))
		}
		return nil
	},
}

// normalizeCommand ensures a command is a "/"-prefixed UCAN command.
func normalizeCommand(cmd string) string {
	if strings.HasPrefix(cmd, "/") {
		return cmd
	}
	return "/" + cmd
}
