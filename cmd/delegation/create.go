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
	"github.com/ipfs/go-cid"
)

var createFlags struct {
	can        []string
	expiration int
	output     string
}

func init() {
	createCmd.Flags().StringArrayVarP(&createFlags.can, "can", "c", nil, "One or more abilities (commands) to delegate, e.g. space/blob/add.")
	createCmd.Flags().IntVarP(&createFlags.expiration, "expiration", "e", 0, "Unix timestamp when the delegation is no longer valid. Zero indicates no expiration.")
	createCmd.Flags().StringVarP(&createFlags.output, "output", "o", "", "Path to write the delegation container to. If not specified, outputs to stdout.")
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
		if len(createFlags.can) == 0 {
			return fmt.Errorf("at least one capability must be specified with --can")
		}

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		space, err := cmdutil.ResolveSpace(c, args[0])
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
		seen := map[cid.Cid]struct{}{}
		var dels []ucan.Delegation
		add := func(d ucan.Delegation) {
			if _, ok := seen[d.Link()]; ok {
				return
			}
			seen[d.Link()] = struct{}{}
			dels = append(dels, d)
		}

		for _, can := range createFlags.can {
			cmdName, err := command.Parse(normalizeCommand(can))
			if err != nil {
				return fmt.Errorf("parsing ability %q: %w", can, err)
			}

			proofs, _, err := c.ProofChain(cmd.Context(), c.Issuer().DID(), cmdName, space)
			if err != nil {
				return fmt.Errorf("building proof chain for %q: %w", can, err)
			}
			if len(proofs) == 0 {
				return fmt.Errorf("no delegations found for ability %q with space %q", can, space)
			}

			del, err := delegation.Delegate(c.Issuer(), aud, space, cmdName, opts...)
			if err != nil {
				return fmt.Errorf("creating delegation: %w", err)
			}
			add(del)
			for _, p := range proofs {
				add(p)
			}
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

// normalizeCommand ensures an ability is a "/"-prefixed UCAN command.
func normalizeCommand(can string) string {
	if strings.HasPrefix(can, "/") {
		return can
	}
	return "/" + can
}
