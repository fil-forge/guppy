package client

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/fil-forge/guppy/internal/ctxutil"
	"github.com/fil-forge/libforge/commands/access"
	"github.com/fil-forge/ucantone/ipld/datamodel"
	"github.com/fil-forge/ucantone/result"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/ipfs/go-cid"
)

// PollClaim attempts to `/access/claim` and retries until it finds delegations
// authorized by way of the given `requestOK`. It returns a channel which will
// produce the result and then close.
func (c *Client) PollClaim(ctx context.Context, requestOK *access.RequestOK) <-chan result.Result[[]ucan.Delegation, error] {
	return c.PollClaimWithTick(ctx, requestOK, time.Tick(time.Second))
}

// PollClaimWithTick is the same as [PollClaim], but accepts the tick channel
// for timing control over the polling. PollClaimWithTick will poll once for
// each value read on `tickChan`, until the claim succeeds or an error occurs.
func (c *Client) PollClaimWithTick(ctx context.Context, requestOK *access.RequestOK, tickChan <-chan time.Time) <-chan result.Result[[]ucan.Delegation, error] {
	resultChan := make(chan result.Result[[]ucan.Delegation, error], 1)

	go func() {
		dlgs, err := c.pollClaimWithTicker(ctx, requestOK, tickChan)
		if err != nil {
			resultChan <- result.Err[[]ucan.Delegation, error](err)
		} else {
			resultChan <- result.OK[[]ucan.Delegation, error](dlgs)
		}
		close(resultChan)
	}()

	return resultChan
}

func (c *Client) pollClaimWithTicker(ctx context.Context, requestOK *access.RequestOK, tickChan <-chan time.Time) ([]ucan.Delegation, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled before delegations could be claimed: %w", ctxutil.Cause(ctx))
		case <-tickChan:
			dels, err := c.ClaimAccess(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to claim access: %w", err)
			}

			// Collect all delegations whose "/access/request" meta matches the
			// request link in requestOK
			relevantDels := make([]ucan.Delegation, 0, len(dels))
			for _, del := range dels {
				var meta datamodel.Map
				if err := meta.UnmarshalCBOR(bytes.NewReader(del.MetadataBytes())); err != nil {
					continue // Skip if metadata can't be parsed for some reason
				}
				requestLinkValue, ok := meta[access.RequestMetaKey]
				if !ok {
					continue // Skip if the meta does not contain "access/request"
				}
				requestLink, ok := requestLinkValue.(cid.Cid)
				if !ok {
					continue // Skip if the meta value is not a valid CID
				}
				if requestLink == requestOK.Request {
					relevantDels = append(relevantDels, del)
				}
			}

			if len(relevantDels) > 0 {
				return relevantDels, nil
			}
		}
	}
}
