package client_test

import (
	"context"
	"fmt"
	"testing"

	spaceblobcap "github.com/fil-forge/go-libstoracha/capabilities/space/blob"
	"github.com/fil-forge/go-libstoracha/capabilities/types"
	libtestutil "github.com/fil-forge/go-libstoracha/testutil"
	"github.com/fil-forge/go-ucanto/core/dag/blockstore"
	"github.com/fil-forge/go-ucanto/core/invocation"
	"github.com/fil-forge/go-ucanto/core/receipt/fx"
	"github.com/fil-forge/go-ucanto/core/result"
	"github.com/fil-forge/go-ucanto/core/result/failure"
	ed25519signer "github.com/fil-forge/go-ucanto/principal/ed25519/signer"
	"github.com/fil-forge/go-ucanto/server"
	uhelpers "github.com/fil-forge/go-ucanto/testing/helpers"
	"github.com/fil-forge/go-ucanto/ucan"
	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/client/testutil"
	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

func TestSpaceBlobReplicate(t *testing.T) {
	t.Run("invokes `space/blob/replicate`", func(t *testing.T) {
		space, err := ed25519signer.Generate()
		require.NoError(t, err)

		invocations := []invocation.Invocation{}
		invokedCapabilities := []ucan.Capability[spaceblobcap.ReplicateCaveats]{}

		connection := testutil.NewTestServerConnection(
			server.WithServiceMethod(
				spaceblobcap.Replicate.Can(),
				server.Provide(
					spaceblobcap.Replicate,
					func(
						ctx context.Context,
						cap ucan.Capability[spaceblobcap.ReplicateCaveats],
						inv invocation.Invocation,
						context server.InvocationContext,
					) (result.Result[spaceblobcap.ReplicateOk, failure.IPLDBuilderFailure], fx.Effects, error) {
						invocations = append(invocations, inv)
						invokedCapabilities = append(invokedCapabilities, cap)
						sitePromises := make([]types.Promise, cap.Nb().Replicas)
						for i := range sitePromises {
							siteDigest, err := multihash.Encode(fmt.Appendf(nil, "test-replicated-site-%d", i), multihash.IDENTITY)
							if err != nil {
								return nil, nil, fmt.Errorf("encoding site digest: %w", err)
							}
							sitePromises[i] = types.Promise{
								UcanAwait: types.Await{
									Selector: ".out.ok.site",
									Link:     cidlink.Link{Cid: cid.NewCidV1(cid.Raw, siteDigest)},
								},
							}
						}
						return result.Ok[spaceblobcap.ReplicateOk, failure.IPLDBuilderFailure](
							spaceblobcap.ReplicateOk{
								Site: sitePromises,
							},
						), nil, nil
					},
				),
			),
		)

		// Act as the space itself for auth simplicity
		c := uhelpers.Must(client.New(client.WithConnection(connection), client.WithPrincipal(space)))

		digest, err := multihash.Encode([]byte("test-digest"), multihash.IDENTITY)
		require.NoError(t, err)

		blob := types.Blob{Digest: digest, Size: 123}

		location := libtestutil.RandomLocationDelegation(t)
		replicateOk, _, err := c.SpaceBlobReplicate(t.Context(), space.DID(), blob, 5, location)
		require.NoError(t, err)

		require.Len(t, invocations, 1, "expected exactly one invocation to be made")
		inv := invocations[0]
		require.Len(t, invokedCapabilities, 1, "expected exactly one capability to be invoked")
		capability := invokedCapabilities[0]

		nb := uhelpers.Must(spaceblobcap.ReplicateCaveatsReader.Read(capability.Nb()))
		require.Equal(t, blob, nb.Blob, "expected to replicate the correct blob")
		require.Equal(t, uint(5), nb.Replicas, "expected to replicate the correct number of replicas")
		require.Equal(t, location.Link(), nb.Site, "expected to replicate from the correct site")

		// Get the location claim from the invocation's extra blocks.
		br, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(inv.Blocks()))
		require.NoError(t, err)
		attachedLocation, err := invocation.NewInvocationView(nb.Site, br)
		require.NoError(t, err)
		require.Equal(t, location.Root().Bytes(), attachedLocation.Root().Bytes(), "expected the invocation to be attached to the location commitment")

		// This is somewhat testing the test, but we want to make sure we get out
		// whatever the server sent.
		require.Len(t, replicateOk.Site, 5, "expected to receive the correct number of site promises")
		for i, p := range replicateOk.Site {
			require.Equal(t, ".out.ok.site", p.UcanAwait.Selector, "expected to receive the correct selector")
			expectedSiteDigest, err := multihash.Encode(fmt.Appendf(nil, "test-replicated-site-%d", i), multihash.IDENTITY)
			require.NoError(t, err)
			expectedSite := cidlink.Link{Cid: cid.NewCidV1(cid.Raw, expectedSiteDigest)}
			require.Equal(t, expectedSite, p.UcanAwait.Link, "expected to receive the correct site promise")
		}
	})
}
