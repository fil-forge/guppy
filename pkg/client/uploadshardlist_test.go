package client_test

import (
	"testing"

	shardcmds "github.com/fil-forge/libforge/commands/upload/shard"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/server"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

func TestUploadShardList(t *testing.T) {
	t.Run("lists shards for an upload", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomIssuer(t)

		results := []cid.Cid{testutil.RandomCID(t), testutil.RandomCID(t)}

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return shardcmds.List.Route(func(req *binding.Request[*shardcmds.ListArguments], res *binding.Response[*shardcmds.ListOK]) error {
					return res.SetSuccess(&shardcmds.ListOK{Size: uint64(len(results)), Results: results})
				})
			}),
		))(t)

		proof := testutil.Must(shardcmds.List.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		page, err := c.UploadShardList(ctx, space.DID(), shardcmds.ListArguments{Root: testutil.RandomCID(t)})
		require.NoError(t, err)
		require.Equal(t, uint64(len(results)), page.Size)
		require.Equal(t, results, page.Results)
	})
}
