package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/briandowns/spinner"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/internal/output"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/guppy/pkg/didmailto"
)

type loginResult struct {
	Account            string `json:"account"`
	LoggedIn           bool   `json:"logged_in"`
	AlreadyLoggedIn    bool   `json:"already_logged_in"`
	ClaimedDelegations int    `json:"claimed_delegations"`
}

var loginCmd = &cobra.Command{
	Use:   "login <account>",
	Short: "Authenticate with a Storacha account",
	Long: wordwrap.WrapString(
		"Authenticates this agent with an email address to gain access to all "+
			"capabilities that have been delegated to it. This command will ask "+
			"Storacha to send an authorization email and then wait for that "+
			"authorization to be confirmed."+
			"\n\n"+
			"You can rerun this command at any time to gain access to any new "+
			"spaces created since your last login. Your agent can authorize with "+
			"multiple Storacha accounts at once; your agent will simply store "+
			"delegations received from each account.",
		80),
	Example: fmt.Sprintf("  %s login racha@storacha.network", rootCmd.Name()),
	Args:    cobra.ExactArgs(1),

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		email := cmd.Flags().Arg(0)

		accountDid, err := didmailto.FromEmail(email)
		if err != nil {
			cmd.SilenceUsage = false
			return err
		}

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return err
		}
		c := cmdutil.MustGetClient(cfg)

		authOk, err := c.RequestAccess(ctx, accountDid)
		if err != nil {
			return fmt.Errorf("requesting access: %w", err)
		}

		// The spinner draws to stdout; in JSON mode stdout is reserved for the
		// result document, so emit the prompt to stderr and skip the spinner.
		if output.IsJSON(cmd) {
			cmd.PrintErrf("🔗 please click the link sent to %s to authorize this agent\n", email)
		} else {
			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond) // Spinner: ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏
			s.Suffix = fmt.Sprintf(" 🔗 please click the link sent to %s to authorize this agent", email)
			s.Start()
			defer s.Stop()
		}

		resultChan := c.PollClaim(ctx, authOk)
		res := <-resultChan
		claim, err := res.Unpack()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return output.Emit(cmd, loginResult{Account: accountDid.String()}, func(w io.Writer) {
				fmt.Fprintln(w, "\nlogin canceled")
			})
		}
		if err != nil {
			return fmt.Errorf("claiming access: %w", err)
		}

		if err := c.AddProofs(ctx, claim.Delegations...); err != nil {
			return fmt.Errorf("adding proofs: %w", err)
		}
		if err := c.AddAttestations(ctx, claim.Attestations...); err != nil {
			return fmt.Errorf("adding attestations: %w", err)
		}

		// Now we should have a proof that allows us to claim delegations for the
		// account we just logged in as. These can then be used to access any spaces
		// the account has access to.
		accountDelegations, _, err := c.ClaimAccess(ctx, accountDid)
		if err != nil {
			return fmt.Errorf("claiming account delegations: %w", err)
		}
		if len(accountDelegations) > 0 {
			log.Infof("Claimed %d delegations for account %s", len(accountDelegations), email)
			if err := c.AddProofs(ctx, accountDelegations...); err != nil {
				return fmt.Errorf("adding proofs: %w", err)
			}
		}

		return output.Emit(cmd, loginResult{
			Account:            accountDid.String(),
			LoggedIn:           true,
			ClaimedDelegations: len(accountDelegations),
		}, func(w io.Writer) {
			fmt.Fprintf(w, "\nSuccessfully logged in as %s!\n", email)
		})
	},
}
