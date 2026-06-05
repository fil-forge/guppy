package client

import (
	"context"
	"fmt"
	"slices"

	accesscmds "github.com/fil-forge/libforge/commands/access"
	attestcmds "github.com/fil-forge/libforge/commands/ucan/attest"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/ipfs/go-cid"
)

// ClaimAccess fetches any stored delegations from the service. This is the
// second step of the Agent authorization process, from the Agent's point of
// view. After the Agent has [RequestAccess]ed, the service will instruct the
// user to confirm the access request out of band, e.g. via email. Once
// confirmed, a delegation will be available on the service for the Agent to
// claim.
func (c *Client) ClaimAccess(ctx context.Context, sub did.DID) ([]ucan.Delegation, []ucan.Invocation, error) {
	var proofs []ucan.Delegation
	var proofLinks []cid.Cid
	var err error
	if c.issuer.DID() != sub {
		proofs, proofLinks, err = c.ProofChain(ctx, c.issuer.DID(), accesscmds.Claim.Command, sub)
		if err != nil {
			return nil, nil, fmt.Errorf("building proof chain: %w", err)
		}
	}

	inv, err := accesscmds.Claim.Invoke(
		c.issuer,
		sub,
		&accesscmds.ClaimArguments{},
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating invocation: %w", err)
	}

	claimOK, _, meta, err := Execute[*accesscmds.ClaimOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithDelegations(proofs...),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("executing claim invocation: %w", err)
	}

	var dlgs []ucan.Delegation
	for _, d := range meta.Delegations() {
		if slices.Contains(claimOK.Delegations, d.Link()) {
			dlgs = append(dlgs, d)
		}
	}
	var attestations []ucan.Invocation
	for _, inv := range meta.Invocations() {
		if inv.Command() != attestcmds.Proof.Command {
			continue
		}
		if inv.Audience() == c.issuer.DID() {
			attestations = append(attestations, inv)
		}
	}

	return dlgs, attestations, nil
}
