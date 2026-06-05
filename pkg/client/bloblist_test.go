package client_test

import (
	"testing"
	"time"

	blobcmds "github.com/fil-forge/libforge/commands/blob"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/server"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

func TestBlobList(t *testing.T) {
	t.Run("lists blobs in a space", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomIssuer(t)

		results := []blobcmds.ListBlobItem{
			{
				Blob:       blobcmds.Blob{Digest: testutil.RandomDigest(t), Size: 32},
				InsertedAt: time.Now().Unix(),
			},
		}

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return blobcmds.List.Route(func(req *binding.Request[*blobcmds.ListArguments], res *binding.Response[*blobcmds.ListOK]) error {
					return res.SetSuccess(&blobcmds.ListOK{Size: uint64(len(results)), Results: results})
				})
			}),
		))(t)

		proof := testutil.Must(blobcmds.List.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		page, err := c.BlobList(ctx, space.DID(), blobcmds.ListArguments{})
		require.NoError(t, err)
		require.Equal(t, uint64(len(results)), page.Size)
		require.Equal(t, results, page.Results)
	})
}
