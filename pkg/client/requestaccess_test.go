package client_test

import (
	"testing"

	accesscmds "github.com/fil-forge/libforge/commands/access"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/server"
	"github.com/fil-forge/ucantone/ucan/command"
	"github.com/fil-forge/ucantone/ucan/promise"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

func TestRequestAccess(t *testing.T) {
	t.Run("invokes `access/request`", func(t *testing.T) {
		ctx := t.Context()
		var captured *accesscmds.RequestArguments
		var invLink cid.Cid

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
				return server.NewRoute(accesscmds.Request, func(req *binding.Request[*accesscmds.RequestArguments], res *binding.Response[*accesscmds.RequestOK]) error {
					captured = req.Task().Arguments()
					invLink = req.Invocation().Link()
					return res.SetSuccess(&accesscmds.RequestOK{
						Request:    req.Invocation().Link(),
						Confirm:    promise.AwaitOK{Task: req.Invocation().Link()},
						Expiration: 123,
					})
				})
			}),
		))(t)

		account := testutil.Must(did.Parse("did:mailto:example.com:alice"))(t)
		authOk, err := c.RequestAccess(ctx, account)
		require.NoError(t, err)

		require.NotNil(t, captured, "expected access/request to be invoked")
		require.Equal(t, account, captured.Issuer, "expected to authorize the correct account")
		require.Len(t, captured.Attenuations, 1, "expected one requested capability")
		require.Equal(t, command.Top(), captured.Attenuations[0].Command, "expected to request all capabilities")

		require.Equal(t, invLink, authOk.Request, "expected to return the request link")
		require.Equal(t, int64(123), authOk.Expiration, "expected to return the expiration")
	})
}
