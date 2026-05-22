package client_test

import (
	"fmt"
	"testing"

	accesscmds "github.com/fil-forge/libforge/commands/access"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/server"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

func TestAccessDelegate(t *testing.T) {
	t.Run("stores delegations via access/delegate", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomSigner(t)
		var receivedSubject did.DID
		var receivedDelegations []cid.Cid

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return server.NewRoute(accesscmds.Delegate, func(req *binding.Request[*accesscmds.DelegateArguments], res *binding.Response[*accesscmds.DelegateOK]) error {
					receivedSubject = req.Invocation().Subject()
					receivedDelegations = req.Task().Arguments().Delegations
					return res.SetSuccess(&accesscmds.DelegateOK{})
				})
			}),
		))(t)

		proof := testutil.Must(accesscmds.Delegate.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		account := testutil.Must(did.Parse("did:mailto:example.com:alice"))(t)
		del := testutil.Must(uploadcmds.Add.Delegate(space, account, space.DID()))(t)

		_, err := c.AccessDelegate(ctx, space.DID(), del)
		require.NoError(t, err)
		require.Equal(t, space.DID(), receivedSubject, "expected access/delegate to be invoked on the space")
		require.Len(t, receivedDelegations, 1, "expected exactly one delegation")
		require.Contains(t, receivedDelegations, del.Link(), "expected the delegation link to be present")
	})

	t.Run("handles multiple delegations", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomSigner(t)
		var receivedDelegations []cid.Cid

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return server.NewRoute(accesscmds.Delegate, func(req *binding.Request[*accesscmds.DelegateArguments], res *binding.Response[*accesscmds.DelegateOK]) error {
					receivedDelegations = req.Task().Arguments().Delegations
					return res.SetSuccess(&accesscmds.DelegateOK{})
				})
			}),
		))(t)

		proof := testutil.Must(accesscmds.Delegate.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		account1 := testutil.Must(did.Parse("did:mailto:example.com:alice"))(t)
		del1 := testutil.Must(uploadcmds.Add.Delegate(space, account1, space.DID()))(t)
		account2 := testutil.Must(did.Parse("did:mailto:example.com:bob"))(t)
		del2 := testutil.Must(uploadcmds.Add.Delegate(space, account2, space.DID()))(t)

		_, err := c.AccessDelegate(ctx, space.DID(), del1, del2)
		require.NoError(t, err)
		require.Len(t, receivedDelegations, 2, "expected exactly two delegations")
		require.Contains(t, receivedDelegations, del1.Link())
		require.Contains(t, receivedDelegations, del2.Link())
	})

	t.Run("includes delegations as proofs in the invocation", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomSigner(t)
		var receivedProofLinks []cid.Cid

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return server.NewRoute(accesscmds.Delegate, func(req *binding.Request[*accesscmds.DelegateArguments], res *binding.Response[*accesscmds.DelegateOK]) error {
					receivedProofLinks = req.Invocation().Proofs()
					return res.SetSuccess(&accesscmds.DelegateOK{})
				})
			}),
		))(t)

		proof := testutil.Must(accesscmds.Delegate.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		account := testutil.Must(did.Parse("did:mailto:example.com:alice"))(t)
		del := testutil.Must(uploadcmds.Add.Delegate(space, account, space.DID()))(t)

		_, err := c.AccessDelegate(ctx, space.DID(), del)
		require.NoError(t, err)
		require.NotEmpty(t, receivedProofLinks, "expected proofs to be included in the invocation")

		foundProof := false
		for _, link := range receivedProofLinks {
			if link == proof.Link() {
				foundProof = true
				break
			}
		}
		assert.True(t, foundProof, "expected the access/delegate authorization proof to be included")
	})

	t.Run("returns error on failure", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomSigner(t)

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return server.NewRoute(accesscmds.Delegate, func(req *binding.Request[*accesscmds.DelegateArguments], res *binding.Response[*accesscmds.DelegateOK]) error {
					return res.SetFailure(fmt.Errorf("test error"))
				})
			}),
		))(t)

		proof := testutil.Must(accesscmds.Delegate.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		account := testutil.Must(did.Parse("did:mailto:example.com:alice"))(t)
		del := testutil.Must(uploadcmds.Add.Delegate(space, account, space.DID()))(t)

		_, err := c.AccessDelegate(ctx, space.DID(), del)
		require.Error(t, err)
	})
}
