package client_test

import (
	"fmt"
	"testing"

	accesscmds "github.com/fil-forge/libforge/commands/access"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/server"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/container"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

func TestClaimAccess(t *testing.T) {
	t.Run("returns the delegations from `access/claim`'s receipt", func(t *testing.T) {
		ctx := t.Context()
		var del ucan.Delegation

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return accesscmds.Claim.Route(func(req *binding.Request[*accesscmds.ClaimArguments], res *binding.Response[*accesscmds.ClaimOK]) error {
					// The claimed delegations travel in the response metadata; the OK
					// payload lists their links.
					if err := res.SetMetadata(container.New(container.WithDelegations(del))); err != nil {
						return err
					}
					return res.SetSuccess(&accesscmds.ClaimOK{Delegations: []cid.Cid{del.Link()}})
				})
			}),
		))(t)

		// Some arbitrary delegation which has been "stored" to be claimed.
		del = testutil.Must(uploadcmds.Add.Delegate(c.Issuer(), c.Issuer().DID(), c.Issuer().DID()))(t)

		claimedDels, err := c.ClaimAccess(ctx)
		require.NoError(t, err)
		require.Len(t, claimedDels, 1, "expected exactly one delegation to be claimed")
		require.Equal(t, del.Link(), claimedDels[0].Link(), "expected the claimed delegation to match the stored one")
	})

	t.Run("returns any handler error", func(t *testing.T) {
		ctx := t.Context()
		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return accesscmds.Claim.Route(func(req *binding.Request[*accesscmds.ClaimArguments], res *binding.Response[*accesscmds.ClaimOK]) error {
					return res.SetFailure(fmt.Errorf("Something went wrong!"))
				})
			}),
		))(t)

		claimedDels, err := c.ClaimAccess(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Something went wrong!")
		require.Len(t, claimedDels, 0)
	})

	t.Run("returns a useful error when the service does not handle access/claim", func(t *testing.T) {
		ctx := t.Context()
		// A server with no routes registered.
		c := testutil.Must(ctestutil.Client(t))(t)

		claimedDels, err := c.ClaimAccess(ctx)
		require.Error(t, err)
		require.Len(t, claimedDels, 0)
	})
}
