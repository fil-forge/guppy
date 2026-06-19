package client_test

import (
	"testing"

	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/testutil"
	"github.com/stretchr/testify/require"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/presets"
	"github.com/fil-forge/guppy/pkg/tokenstore"
)

// NOTE: the old Proofs(CapabilityQuery) wildcard-matching API and the
// WithStore / WithPrincipal / WithAdditionalProofs options were removed in the
// ucantone migration — proof selection now lives in the token store's
// ProofChain (exercised in libforge's own tests). TODO(forrest): the removed
// TestProofs / TestWithAdditionalProofs coverage should be re-homed against the
// new ProofChain model if/when that selection logic lives in guppy again.

func TestReset(t *testing.T) {
	ctx := t.Context()
	agent := testutil.RandomMultikeyIssuer(t)
	store := tokenstore.NewMemStore()
	c := testutil.Must(client.New(
		agent,
		presets.DefaultNetwork.UploadID,
		presets.DefaultNetwork.UploadURL,
		client.WithTokenStore(store),
	))(t)

	// No proofs initially.
	proofs, _, err := c.ProofChain(ctx, agent.DID(), uploadcmds.Add.Command, agent.DID())
	require.NoError(t, err)
	require.Empty(t, proofs, "expected no proofs initially")

	// Add an arbitrary self-issued delegation.
	del := testutil.Must(uploadcmds.Add.Delegate(agent, agent.DID(), agent.DID()))(t)
	require.NoError(t, c.AddProofs(ctx, del))

	proofs, _, err = c.ProofChain(ctx, agent.DID(), uploadcmds.Add.Command, agent.DID())
	require.NoError(t, err)
	require.Len(t, proofs, 1, "expected the added delegation to be returned")

	// Reset clears the store but preserves the agent identity.
	require.NoError(t, c.Reset(ctx))
	proofs, _, err = c.ProofChain(ctx, agent.DID(), uploadcmds.Add.Command, agent.DID())
	require.NoError(t, err)
	require.Empty(t, proofs, "expected all proofs to be removed after reset")
	require.Equal(t, agent.DID(), c.DID(), "expected agent DID to be unchanged after reset")
}
