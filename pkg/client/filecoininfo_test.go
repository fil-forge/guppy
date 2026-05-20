package client_test

import (
	"context"
	"testing"

	filecoincap "github.com/fil-forge/go-libstoracha/capabilities/filecoin"
	"github.com/fil-forge/go-ucanto/core/delegation"
	"github.com/fil-forge/go-ucanto/core/invocation"
	"github.com/fil-forge/go-ucanto/core/receipt/fx"
	"github.com/fil-forge/go-ucanto/core/result"
	"github.com/fil-forge/go-ucanto/core/result/failure"
	ed25519signer "github.com/fil-forge/go-ucanto/principal/ed25519/signer"
	"github.com/fil-forge/go-ucanto/server"
	"github.com/fil-forge/go-ucanto/testing/helpers"
	"github.com/fil-forge/go-ucanto/ucan"
	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/client/testutil"
	"github.com/stretchr/testify/require"
)

func TestFilecoinInfo(t *testing.T) {
	space, err := ed25519signer.Generate()
	require.NoError(t, err)

	pieceLink := helpers.RandomCID()

	invokedCapabilities := []ucan.Capability[filecoincap.InfoCaveats]{}

	connection := testutil.NewTestServerConnection(
		server.WithServiceMethod(
			filecoincap.Info.Can(),
			server.Provide(
				filecoincap.Info,
				func(
					ctx context.Context,
					cap ucan.Capability[filecoincap.InfoCaveats],
					inv invocation.Invocation,
					context server.InvocationContext,
				) (result.Result[filecoincap.InfoOk, failure.IPLDBuilderFailure], fx.Effects, error) {
					invokedCapabilities = append(invokedCapabilities, cap)
					return result.Ok[filecoincap.InfoOk, failure.IPLDBuilderFailure](
						filecoincap.InfoOk{
							Piece: cap.Nb().Piece,
						},
					), nil, nil
				},
			),
		),
	)

	c := helpers.Must(client.New(client.WithConnection(connection)))

	cap := ucan.NewCapability("*", space.DID().String(), ucan.NoCaveats{})
	proof, err := delegation.Delegate(space, c.Issuer(), []ucan.Capability[ucan.NoCaveats]{cap}, delegation.WithNoExpiration())
	require.NoError(t, err)
	err = c.AddProofs(proof)
	require.NoError(t, err)

	infoOk, err := c.FilecoinInfo(t.Context(), space.DID(), pieceLink)
	require.NoError(t, err)

	require.Len(t, invokedCapabilities, 1, "expected exactly one capability to be invoked")
	capability := invokedCapabilities[0]

	require.Equal(t, space.DID().String(), capability.With(), "expected to have the space as the resource")
	require.Equal(t, pieceLink, capability.Nb().Piece, "expected to have the correct piece link")
	require.Equal(t, pieceLink, infoOk.Piece, "expected to get the correct piece link in the response")
}
