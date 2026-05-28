package client

import (
	"context"
	"fmt"
	"slices"

	"github.com/fil-forge/libforge/commands/access"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/invocation"
)

// ClaimAccess fetches any stored delegations from the service. This is the
// second step of the Agent authorization process, from the Agent's point of
// view. After the Agent has [RequestAccess]ed, the service will instruct the
// user to confirm the access request out of band, e.g. via email. Once
// confirmed, a delegation will be available on the service for the Agent to
// claim.
func (c *Client) ClaimAccess(ctx context.Context) ([]ucan.Delegation, error) {
	inv, err := access.Claim.Invoke(
		c.signer,
		c.signer.DID(),
		&access.ClaimArguments{},
		invocation.WithAudience(c.serviceID),
	)
	if err != nil {
		return nil, fmt.Errorf("creating invocation: %w", err)
	}

	claimOK, _, meta, err := Execute[*access.ClaimOK](ctx, c.ucanClient, inv)
	if err != nil {
		return nil, fmt.Errorf("executing claim invocation: %w", err)
	}

	// Persist the service-issued attestations alongside the delegations.
	// Delegations from did:mailto accounts use a non-standard signature
	// (the account can't actually sign — it's an absentee signer), so
	// every subsequent invocation that uses such a delegation as a proof
	// must also present an `/ucan/attest/proof` invocation from the
	// service. ProofAttestations later looks these up from the token
	// store; if we don't persist them here, that lookup fails with
	// "no attestation found for proof signed by did:mailto:..." the
	// first time the agent tries to act on the account's behalf.
	if invs := meta.Invocations(); len(invs) > 0 {
		if err := c.tokenStore.AddInvocations(ctx, invs...); err != nil {
			return nil, fmt.Errorf("storing attestations: %w", err)
		}
	}

	var dlgs []ucan.Delegation
	for _, d := range meta.Delegations() {
		if slices.Contains(claimOK.Delegations, d.Link()) {
			dlgs = append(dlgs, d)
		}
	}

	return dlgs, nil
}
