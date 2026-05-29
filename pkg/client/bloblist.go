package client

import (
	"context"
	"fmt"

	blobcmds "github.com/fil-forge/libforge/commands/blob"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan/invocation"
)

// BlobList returns a paginated list of blobs stored in a space.
//
// Required delegated capability proofs: `/blob/list`
//
// The `space` is the resource the invocation applies to. It is typically the
// DID of a space.
//
// The `args` are arguments required to perform an `/blob/list` invocation.
func (c *Client) BlobList(ctx context.Context, space did.DID, args blobcmds.ListArguments) (*blobcmds.ListOK, error) {
	proofs, proofLinks, err := c.ProofChain(ctx, c.signer.DID(), blobcmds.List.Command, space)
	if err != nil {
		return nil, fmt.Errorf("building proof chain: %w", err)
	}
	attestations, err := c.ProofAttestations(ctx, proofs, c.serviceID)
	if err != nil {
		return nil, fmt.Errorf("fetching proof attestations: %w", err)
	}

	inv, err := blobcmds.List.Invoke(
		c.signer,
		space,
		&args,
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating invocation: %w", err)
	}

	listOK, _, _, err := Execute[*blobcmds.ListOK](
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
