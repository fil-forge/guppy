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

type ClaimResult struct {
	Delegations  []ucan.Delegation
	Attestations []ucan.Invocation
}

// PollClaim attempts to `/access/claim` and retries until it finds delegations
// authorized by way of the given `requestOK`. It returns a channel which will
// produce the result and then close.
func (c *Client) PollClaim(ctx context.Context, requestOK *access.RequestOK) <-chan result.Result[ClaimResult, error] {
	return c.PollClaimWithTick(ctx, requestOK, time.Tick(time.Second))
}

// PollClaimWithTick is the same as [PollClaim], but accepts the tick channel
// for timing control over the polling. PollClaimWithTick will poll once for
// each value read on `tickChan`, until the claim succeeds or an error occurs.
func (c *Client) PollClaimWithTick(ctx context.Context, requestOK *access.RequestOK, tickChan <-chan time.Time) <-chan result.Result[ClaimResult, error] {
	resultChan := make(chan result.Result[ClaimResult, error], 1)

	go func() {
		dlgs, attestations, err := c.pollClaimWithTicker(ctx, requestOK, tickChan)
		if err != nil {
			resultChan <- result.Err[ClaimResult](err)
		} else {
			resultChan <- result.OK[ClaimResult, error](ClaimResult{
				Delegations:  dlgs,
				Attestations: attestations,
			})
		}
		close(resultChan)
	}()

	return resultChan
}

func (c *Client) pollClaimWithTicker(ctx context.Context, requestOK *access.RequestOK, tickChan <-chan time.Time) ([]ucan.Delegation, []ucan.Invocation, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("context canceled before delegations could be claimed: %w", ctxutil.Cause(ctx))
		case <-tickChan:
			dels, attestations, err := c.ClaimAccess(ctx, c.issuer.DID())
			if err != nil {
				return nil, nil, fmt.Errorf("failed to claim access: %w", err)
			}

			// Collect all delegations whose "/access/request" meta matches the
			// request link in requestOK
			relevantDels := make([]ucan.Delegation, 0, len(dels))
			for _, del := range dels {
				if isRequestedToken(requestOK.Request, del.MetadataBytes()) {
					relevantDels = append(relevantDels, del)
				}
			}

			relevantAttestations := make([]ucan.Invocation, 0, len(attestations))
			for _, att := range attestations {
				if isRequestedToken(requestOK.Request, att.MetadataBytes()) {
					relevantAttestations = append(relevantAttestations, att)
				}
			}

			if len(relevantDels) > 0 {
				return relevantDels, relevantAttestations, nil
			}
		}
	}
}

func isRequestedToken(requestLink cid.Cid, metaBytes []byte) bool {
	var meta datamodel.Map
	if err := meta.UnmarshalCBOR(bytes.NewReader(metaBytes)); err != nil {
		return false // Skip if metadata can't be parsed for some reason
	}
	metaRequestLinkValue, ok := meta[access.RequestMetaKey]
	if !ok {
		return false // Skip if the meta does not contain [access.RequestMetaKey] key
	}
	metaRequestLink, ok := metaRequestLinkValue.(cid.Cid)
	if !ok {
		return false // Skip if the meta value is not a valid CID
	}
	return metaRequestLink == requestLink
}
