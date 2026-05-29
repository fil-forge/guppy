package space

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"slices"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/guppy/pkg/didmailto"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/ucan/command"
	"github.com/fil-forge/ucantone/ucan/delegation"
)

type spaceGenerateResult struct {
	DID string `json:"did"`
}

var generateFlags struct {
	name        string
	grantTo     string
	provisionTo string
	outputKey   bool
}

func init() {
	generateCmd.Flags().StringVar(&generateFlags.name, "name", "", "Name for the space (optional)")
	generateCmd.Flags().StringVar(&generateFlags.grantTo, "grant-to", "", "Account DID to grant space access to. Must be logged in already. (optional when exactly one account is logged in)")
	generateCmd.Flags().StringVar(&generateFlags.provisionTo, "provision-to", "", "Account DID to provision space to. Must be logged in already. (optional when exactly one account is logged in)")
	generateCmd.Flags().BoolVarP(&generateFlags.outputKey, "output-key", "k", false, "Output the space key (WARNING: sensitive data)")
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new space",
	Long: wordwrap.WrapString(
		"Generates a new Storacha space, provisions it to the logged-in account, "+
			"grants space access to the logged-in account, and stores it in the "+
			"local store.",
		80),
	RunE: func(cmd *cobra.Command, args []string) error {
		space, err := ed25519.Generate()
		if err != nil {
			cmd.SilenceUsage = false
			return fmt.Errorf("generating signer for space: %w", err)
		}

		if generateFlags.outputKey {
			cmd.PrintErrln("\nWARNING: This is your space private key. Keep it secret and secure!")
			cmd.PrintErrln("Space Key (base64):")
			// Never write the secret to stdout in JSON mode — stdout must stay a
			// clean JSON document.
			if output.IsJSON(cmd) {
				cmd.PrintErrln(base64.StdEncoding.EncodeToString(space.Raw()))
			} else {
				fmt.Println(base64.StdEncoding.EncodeToString(space.Raw()))
			}
			cmd.PrintErrln()
		}

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}

		c := cmdutil.MustGetClient(cfg)
		accounts, err := c.Accounts(cmd.Context())
		if err != nil {
			return err
		}

		provisionAccount, err := pickAccount(cmd, accounts, generateFlags.provisionTo, "provision-to")
		if err != nil {
			return err
		}
		grantAccount, err := pickAccount(cmd, accounts, generateFlags.grantTo, "grant-to")
		if err != nil {
			return err
		}

		cmd.PrintErrf("Provisioning %s to %s...\n\n", space.DID(), provisionAccount)
		if _, err := c.ProviderAdd(cmd.Context(), provisionAccount, c.ServiceID(), space.DID()); err != nil {
			return fmt.Errorf("provisioning space: %w", err)
		}

		cmd.PrintErrf("Granting access on %s to %s...\n\n", space.DID(), grantAccount)
		if err := grant(cmd.Context(), c, space, grantAccount, generateFlags.name); err != nil {
			return fmt.Errorf("granting capabilities: %w", err)
		}

		cmd.PrintErr("Generated space: ")
		// all other output is on stderr; only the space DID (text) or a single
		// JSON document goes to stdout, allowing: export SPACE=$(guppy space generate)
		err = output.Emit(cmd, spaceGenerateResult{DID: space.DID().String()}, func(w io.Writer) {
			fmt.Fprint(w, space.DID().String())
		})
		cmd.PrintErr("\n\n")
		return err
	},
}

// pickAccount resolves the account for an operation: the explicit flag value if
// given (and logged in), otherwise the sole logged-in account.
func pickAccount(cmd *cobra.Command, accounts []did.DID, flagValue, flagName string) (did.DID, error) {
	if flagValue != "" {
		account, err := didmailto.FromInput(flagValue)
		if err != nil {
			cmd.SilenceUsage = false
			return did.Undef, fmt.Errorf("parsing `--%s` account %q: %w", flagName, flagValue, err)
		}
		if !slices.Contains(accounts, account) {
			cmd.PrintErrf("Account %s is not logged in yet. Use `guppy login %s` to log in.\n", flagValue, flagValue)
			return did.Undef, cmdutil.NewHandledCliError(fmt.Errorf("account %s is not logged in", account))
		}
		return account, nil
	}

	switch len(accounts) {
	case 0:
		cmd.PrintErrf("No accounts are logged in yet. Use `guppy login <account>` to log in.\n")
		return did.Undef, cmdutil.NewHandledCliError(fmt.Errorf("no accounts are logged in"))
	case 1:
		return accounts[0], nil
	default:
		var b string
		for _, acct := range accounts {
			b += fmt.Sprintf("- %s\n", acct)
		}
		cmd.PrintErrf("Multiple accounts are logged in.\n%s\nSpecify an account with `--%s`.\n", b, flagName)
		return did.Undef, cmdutil.NewHandledCliError(fmt.Errorf("multiple accounts are logged in"))
	}
}

// grant delegates full access to the space to both the agent (so it can act on
// the space locally) and the account (stored on the service via access/delegate),
// recording the space's name in the delegation metadata.
func grant(ctx context.Context, c *client.Client, space ed25519.Signer, account did.DID, name string) error {
	opts := []delegation.Option{delegation.WithNoExpiration()}
	if name != "" {
		opts = append(opts, delegation.WithMetadata(client.SpaceNameMetadata(name)))
	}

	agentGrant, err := delegation.Delegate(space, c.Issuer().DID(), space.DID(), command.Top(), opts...)
	if err != nil {
		return fmt.Errorf("creating agent delegation: %w", err)
	}
	accountGrant, err := delegation.Delegate(space, account, space.DID(), command.Top(), opts...)
	if err != nil {
		return fmt.Errorf("creating account delegation: %w", err)
	}

	// Keep both locally so the agent can act, then register the account's grant
	// with the service.
	if err := c.AddProofs(ctx, agentGrant, accountGrant); err != nil {
		return fmt.Errorf("storing delegations: %w", err)
	}
	if _, err := c.AccessDelegate(ctx, space.DID(), accountGrant); err != nil {
		return fmt.Errorf("storing delegation via access/delegate: %w", err)
	}
	return nil
}
