package client

import (
	"context"
	"fmt"
	"io"
	"math/rand"

	"github.com/fil-forge/guppy/internal/ctxutil"
	"github.com/fil-forge/guppy/pkg/client/locator"
	contentcmds "github.com/fil-forge/libforge/commands/content"
	"github.com/fil-forge/libforge/ucan/retrieval"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan/invocation"
)

func (c *Client) Retrieve(ctx context.Context, location locator.Location) (io.ReadCloser, error) {
	space := location.Commitment.Space
	urls := location.Commitment.Location
	url := urls[rand.Intn(len(urls))]

	proofs, proofLinks, err := c.ProofChain(ctx, c.issuer.DID(), contentcmds.Retrieve.Command, space)
	if err != nil {
		return nil, fmt.Errorf("building proof chain: %w", err)
	}

	inv, err := contentcmds.Retrieve.Invoke(
		c.Issuer(),
		space,
		&contentcmds.RetrieveArguments{
			Blob: contentcmds.Blob{Digest: location.Commitment.Content},
			Range: contentcmds.Range{
				Start: uint64(location.Range.Start),
				End:   uint64(location.Range.End),
			},
		},
		invocation.WithAudience(location.Commitment.Node),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, fmt.Errorf("invoking `space/content/retrieve`: %w", err)
	}

	client, err := retrieval.NewClient(url.URL(), c.retrievalOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating retrieval client: %w", err)
	}

	_, _, meta, err := Execute[*contentcmds.RetrieveOK](
		ctx,
		client,
		inv,
		execution.WithDelegations(proofs...),
	)
	if err != nil {
		return nil, fmt.Errorf("executing invocation: %w", ctxutil.EnrichWithCause(err, ctx))
	}

	hcRes, ok := meta.(*retrieval.HTTPHeaderResponseContainer)
	if !ok {
		return nil, fmt.Errorf("unexpected metadata type: %T", meta)
	}
	log.Debugw("retrieved content", "status", hcRes.StatusCode, "headers", hcRes.Header)

	return hcRes.Body, nil
}
