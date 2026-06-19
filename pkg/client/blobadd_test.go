package client_test

import (
	"bytes"
	"testing"

	blobcmds "github.com/fil-forge/libforge/commands/blob"
	"github.com/fil-forge/libforge/testutil"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/fil-forge/guppy/pkg/client"
	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

func TestBlobAdd(t *testing.T) {
	t.Run("adds and PUTs the blob", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomIssuer(t)
		putClient := ctestutil.NewPutClient()

		c := testutil.Must(ctestutil.Client(t, ctestutil.WithBlobAdd(t)))(t)

		proof := testutil.Must(blobcmds.Add.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		addedBlob, err := c.BlobAdd(ctx, bytes.NewReader([]byte("test")), space.DID(), client.WithPutClient(putClient))
		require.NoError(t, err)

		digest, err := multihash.Sum([]byte("test"), multihash.SHA2_256, -1)
		require.NoError(t, err)

		require.Equal(t, digest, addedBlob.Digest)
		require.Equal(t, []byte("test"), ctestutil.ReceivedBlobs(putClient).Get(digest))
		require.Equal(t, 1, ctestutil.ReceivedBlobs(putClient).Size())
		// The mock blob/add flow always issues a pdp/accept.
		require.NotNil(t, addedBlob.PDPAccept)
	})

	t.Run("with a pre-existing http/put success receipt, does not PUT", func(t *testing.T) {
		ctx := t.Context()
		space := testutil.RandomIssuer(t)
		putClient := ctestutil.NewPutClient()

		c := testutil.Must(ctestutil.Client(t, ctestutil.WithBlobAddPutReceipt(t)))(t)

		proof := testutil.Must(blobcmds.Add.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(ctx, proof))

		addedBlob, err := c.BlobAdd(ctx, bytes.NewReader([]byte("test")), space.DID(), client.WithPutClient(putClient))
		require.NoError(t, err)

		digest, err := multihash.Sum([]byte("test"), multihash.SHA2_256, -1)
		require.NoError(t, err)

		require.Equal(t, digest, addedBlob.Digest)
		// The blob must NOT have been PUT because a success receipt was already present.
		require.Equal(t, 0, ctestutil.ReceivedBlobs(putClient).Size())
	})
}
