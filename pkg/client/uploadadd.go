package client

import (
	"context"
	"fmt"

	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/ipfs/go-cid"
)

// UploadAdd registers an "upload" with the service. The issuer needs proof of
// `/upload/add` delegated capability.
//
// Required delegated capability proofs: `/upload/add`
//
// The `space` is the resource the invocation applies to. It is typically the
// DID of a space.
func (c *Client) UploadAdd(ctx context.Context, space did.DID, root cid.Cid, shards []cid.Cid, index *cid.Cid) (*uploadcmds.AddOK, error) {
	proofs, proofLinks, err := c.ProofChain(ctx, c.signer.DID(), uploadcmds.Add.Command, space)
	if err != nil {
		return nil, fmt.Errorf("building proof chain: %w", err)
	}
	attestations, err := c.ProofAttestations(ctx, proofs, c.serviceID)
	if err != nil {
		return nil, fmt.Errorf("fetching proof attestations: %w", err)
	}
	inv, err := uploadcmds.Add.Invoke(
		c.signer,
		space,
		&uploadcmds.AddArguments{
			Root:   root,
			Shards: shards,
			Index:  index,
		},
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating invocation: %w", err)
	}

	addOK, _, _, err := Execute[*uploadcmds.AddOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithDelegations(proofs...),
		execution.WithInvocations(attestations...),
	)
	if err != nil {
		return nil, fmt.Errorf("executing invocation: %w", err)
	}

	return addOK, nil
}
