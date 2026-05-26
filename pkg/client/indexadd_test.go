package client_test

import (
	"testing"

	contentcmds "github.com/fil-forge/libforge/commands/content"
	indexcmds "github.com/fil-forge/libforge/commands/index"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/server"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

// NOTE: the old SpaceIndexAdd(index, size, root, space) signature became
// IndexAdd(index, space). The size / content-root and the size-derived retrieval
// auth range are gone, so this test asserts only the index/add invocation.
// TODO(forrest): extend if IndexAdd grows richer arguments again.

func TestIndexAdd(t *testing.T) {
	t.Run("invokes index/add for the given index", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomSigner(t)

		var capturedIndex cid.Cid
		var capturedSubject did.DID

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return indexcmds.Add.Route(func(req *binding.Request[*indexcmds.AddArguments], res *binding.Response[*indexcmds.AddOK]) error {
					capturedIndex = req.Task().Arguments().Index
					capturedSubject = req.Invocation().Subject()
					return res.SetSuccess(&indexcmds.AddOK{})
				})
			}),
		))(t)

		// IndexAdd builds a content/retrieve authorization and invokes index/add,
		// so the agent needs proofs for both commands on the space.
		require.NoError(t, c.AddProofs(ctx,
			testutil.Must(contentcmds.Retrieve.Delegate(space, c.Issuer().DID(), space.DID()))(t),
			testutil.Must(indexcmds.Add.Delegate(space, c.Issuer().DID(), space.DID()))(t),
		))

		indexCID := testutil.RandomCID(t)
		require.NoError(t, c.IndexAdd(ctx, indexCID, space.DID()))

		require.Equal(t, indexCID, capturedIndex, "expected index/add to carry the index CID")
		require.Equal(t, space.DID(), capturedSubject, "expected index/add to be invoked on the space")
	})
}
