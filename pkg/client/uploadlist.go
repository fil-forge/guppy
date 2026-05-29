package client

import (
	"context"
	"fmt"

	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan/invocation"
)

// UploadList returns a paginated list of uploads in a space.
//
// Required delegated capability proofs: `/upload/list`
//
// The `space` is the resource the invocation applies to. It is typically the
// DID of a space.
func (c *Client) UploadList(ctx context.Context, space did.DID, args uploadcmds.ListArguments) (*uploadcmds.ListOK, error) {
	proofs, proofLinks, err := c.ProofChain(ctx, c.signer.DID(), uploadcmds.Add.Command, space)
	if err != nil {
		return nil, fmt.Errorf("building proof chain: %w", err)
	}
	attestations, err := c.ProofAttestations(ctx, proofs, c.serviceID)
	if err != nil {
		return nil, fmt.Errorf("fetching proof attestations: %w", err)
	}
	inv, err := uploadcmds.List.Invoke(
		c.signer,
		space,
		&args,
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating invocation: %w", err)
	}

	listOK, _, _, err := Execute[*uploadcmds.ListOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithDelegations(proofs...),
		execution.WithInvocations(attestations...),
	)
	if err != nil {
		return nil, fmt.Errorf("executing invocation: %w", err)
	}
	return listOK, nil
}
