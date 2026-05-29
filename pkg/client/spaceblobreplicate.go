package client

/*
import (
	"context"
	"fmt"

	spaceblobcap "github.com/fil-forge/go-libstoracha/capabilities/space/blob"
	"github.com/fil-forge/go-libstoracha/capabilities/types"
	"github.com/fil-forge/go-ucanto/core/delegation"
	"github.com/fil-forge/go-ucanto/core/receipt/fx"
	"github.com/fil-forge/go-ucanto/core/result"
	"github.com/fil-forge/ucantone/did"
)

func (c *Client) SpaceBlobReplicate(ctx context.Context, space did.DID, blob types.Blob, replicaCount uint, locationCommitment delegation.Delegation) (spaceblobcap.ReplicateOk, fx.Effects, error) {
	caveats := spaceblobcap.ReplicateCaveats{
		Blob:     blob,
		Replicas: replicaCount,
		Site:     locationCommitment.Link(),
	}

	inv, err := invoke[spaceblobcap.ReplicateCaveats, spaceblobcap.ReplicateOk](
		c,
		spaceblobcap.Replicate,
		space.String(),
		caveats,
	)
	if err != nil {
		return spaceblobcap.ReplicateOk{}, nil, fmt.Errorf("invoking `space/blob/replicate`: %w", err)
	}
	for b, err := range locationCommitment.Blocks() {
		if err != nil {
			return spaceblobcap.ReplicateOk{}, nil, fmt.Errorf("getting block from location commitment: %w", err)
		}
		inv.Attach(b)
	}

	res, fx, err := execute[spaceblobcap.ReplicateCaveats, spaceblobcap.ReplicateOk](
		ctx,
		c,
		spaceblobcap.Replicate,
		inv,
		spaceblobcap.ReplicateOkType(),
	)
	if err != nil {
		return spaceblobcap.ReplicateOk{}, nil, fmt.Errorf("executing `space/blob/replicate`: %w", err)
	}

	replicateOk, failErr := result.Unwrap(res)
	if failErr != nil {
		return spaceblobcap.ReplicateOk{}, nil, fmt.Errorf("`space/blob/replicate` failed: %w", failErr)
	}

	return replicateOk, fx, nil
}
*/
