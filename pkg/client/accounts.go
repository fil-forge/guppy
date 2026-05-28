package client

import (
	"cmp"
	"context"
	"maps"
	"slices"

	"github.com/fil-forge/ucantone/did"
)

// Accounts returns the set of did:mailto accounts the agent is currently
// authorized to act on behalf of. An account is considered "logged in" iff
// the store holds a delegation issued by it whose audience is this agent.
//
// We deliberately scan all delegations rather than narrowly query by
// (aud, cmd, sub) — sprue's access/confirm now issues root delegations with
// subject == account.DID() (not did.Undef / powerline), so a narrow query
// keyed on a specific subject form would miss them. The set of did:mailto
// issuers is small in practice (the user's logged-in accounts), so the
// full-scan cost is negligible.
func (c *Client) Accounts(ctx context.Context) ([]did.DID, error) {
	dlgs, err := c.tokenStore.Delegations(ctx)
	if err != nil {
		return nil, err
	}
	accounts := make(map[did.DID]struct{})
	for _, d := range dlgs {
		if d.Audience() != c.signer.DID() {
			continue
		}
		if d.Issuer().Method() != "mailto" {
			continue
		}
		accounts[d.Issuer()] = struct{}{}
	}
	result := slices.Collect(maps.Keys(accounts))
	slices.SortFunc(result, func(a, b did.DID) int {
		return cmp.Compare(a.String(), b.String())
	})
	return result, nil
}
