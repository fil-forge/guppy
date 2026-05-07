package client

import (
	"context"
	"fmt"

	uploadcap "github.com/fil-forge/go-libstoracha/capabilities/upload"
	"github.com/fil-forge/go-ucanto/core/result"
	"github.com/fil-forge/go-ucanto/did"
	"github.com/ipld/go-ipld-prime"
)

// UploadAdd registers an "upload" with the service. The issuer needs proof of
// `upload/add` delegated capability.
//
// Required delegated capability proofs: `upload/add`
//
// The `space` is the resource the invocation applies to. It is typically the
// DID of a space.
//
// The `proofs` are delegation proofs to use in addition to those in the client.
// They won't be saved in the client, only used for this invocation.
//
// The `caveats` are caveats required to perform an `upload/add` invocation.
func (c *Client) UploadAdd(ctx context.Context, space did.DID, root ipld.Link, shards []ipld.Link) (uploadcap.AddOk, error) {
	res, _, err := invokeAndExecute[uploadcap.AddCaveats, uploadcap.AddOk](
		ctx,
		c,
		uploadcap.Add,
		space.String(),
		uploadcap.AddCaveats{
			Root:   root,
			Shards: shards,
		},
		uploadcap.AddOkType(),
	)

	if err != nil {
		return uploadcap.AddOk{}, fmt.Errorf("invoking and executing `upload/add`: %w", err)
	}

	addOk, failErr := result.Unwrap(res)
	if failErr != nil {
		return uploadcap.AddOk{}, fmt.Errorf("`upload/add` failed: %w", failErr)
	}

	return addOk, nil
}
