package client

import (
	"context"
	"fmt"

	providercmds "github.com/fil-forge/libforge/commands/provider"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan/invocation"
)

// ProviderAdd invokes the /provider/add capability to provision a space with a
// customer account.
func (c *Client) ProviderAdd(ctx context.Context, customerAccount did.DID, provider did.DID, consumer did.DID) (*providercmds.AddOK, error) {
	proofs, proofLinks, err := c.ProofChain(ctx, c.issuer.DID(), providercmds.Add.Command, customerAccount)
	if err != nil {
		return nil, fmt.Errorf("building proof chain: %w", err)
	}

	inv, err := providercmds.Add.Invoke(
		c.issuer,
		customerAccount,
		&providercmds.AddArguments{
			Provider: provider,
			Consumer: consumer,
		},
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating invocation: %w", err)
	}

	addOK, _, _, err := Execute[*providercmds.AddOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithDelegations(proofs...),
	)
	if err != nil {
		return nil, fmt.Errorf("executing provider add invocation: %w", err)
	}

	return addOK, nil
}
