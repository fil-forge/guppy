package client

import (
	"context"
	"fmt"

	spacecmds "github.com/fil-forge/libforge/commands/space"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan/invocation"
)

// SpaceInfo invokes the /space/info capability to get information about a space,
// including which providers are associated with it.
func (c *Client) SpaceInfo(ctx context.Context, space did.DID) (*spacecmds.InfoOK, error) {
	proofs, proofLinks, err := c.ProofChain(ctx, c.signer.DID(), spacecmds.Info.Command, space)
	if err != nil {
		return nil, fmt.Errorf("building proof chain: %w", err)
	}
	attestations, err := c.ProofAttestations(ctx, proofs, c.serviceID)
	if err != nil {
		return nil, fmt.Errorf("fetching proof attestations: %w", err)
	}

	inv, err := spacecmds.Info.Invoke(
		c.signer,
		space,
		&spacecmds.InfoArguments{},
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating invocation: %w", err)
	}

	infoOK, _, _, err := Execute[*spacecmds.InfoOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithDelegations(proofs...),
		execution.WithInvocations(attestations...),
	)
	if err != nil {
		return nil, fmt.Errorf("executing invocation: %w", err)
	}

	return infoOK, nil
}
