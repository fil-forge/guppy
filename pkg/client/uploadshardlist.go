package client

import (
	"context"
	"fmt"

	shardcmds "github.com/fil-forge/libforge/commands/upload/shard"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan/invocation"
)

// UploadShardList returns a paginated list of shards for an upload.
//
// Required delegated capability proofs: `upload/shard/list`
//
// The `space` is the resource the invocation applies to. It is typically the
// DID of a space.
func (c *Client) UploadShardList(ctx context.Context, space did.DID, args shardcmds.ListArguments) (*shardcmds.ListOK, error) {
	proofs, proofLinks, err := c.ProofChain(ctx, c.issuer.DID(), shardcmds.List.Command, space)
	if err != nil {
		return nil, fmt.Errorf("building proof chain: %w", err)
	}
	inv, err := shardcmds.List.Invoke(
		c.issuer,
		space,
		&args,
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating invocation: %w", err)
	}

	listOK, _, _, err := Execute[*shardcmds.ListOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithDelegations(proofs...),
	)
	if err != nil {
		return nil, fmt.Errorf("executing invocation: %w", err)
	}
	return listOK, nil
}
