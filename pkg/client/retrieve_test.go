package client_test

import (
	"io"
	"net/url"
	"testing"

	"github.com/fil-forge/libforge/blobindex"
	"github.com/fil-forge/libforge/commands"
	contentcmds "github.com/fil-forge/libforge/commands/content"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/libforge/ucan/retrieval"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/client/locator"
	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

func TestRetrieve(t *testing.T) {
	t.Run("successfully retrieves content", func(t *testing.T) {
		testData := []byte("Hello, world! This is test data for retrieval.")
		dataHash, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		require.NoError(t, err)

		space := testutil.RandomIssuer(t)
		storageProvider := testutil.RandomIssuer(t)

		// In-process retrieval server that serves testData.
		httpClient := ctestutil.NewRetrievalClient(t, storageProvider, testData)

		serverURL := testutil.Must(url.Parse("https://storage1.example.com/blob/" + digestutil.Format(dataHash)))(t)

		c := testutil.Must(ctestutil.Client(t,
			ctestutil.WithClientOptions(client.WithRetrievalOptions(retrieval.WithHTTPClient(httpClient))),
		))(t)

		proof := testutil.Must(contentcmds.Retrieve.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(t.Context(), proof))

		location := locator.Location{
			Commitment: locator.Commitment{
				Node:     storageProvider.DID(),
				Space:    space.DID(),
				Content:  dataHash,
				Location: []commands.CborURL{commands.CborURL(*serverURL)},
			},
			Range: blobindex.Range{Start: 0, End: int64(len(testData)) - 1},
		}

		dataReader, err := c.Retrieve(t.Context(), location)
		require.NoError(t, err)
		data, err := io.ReadAll(dataReader)
		require.NoError(t, err)
		dataReader.Close()
		require.Equal(t, testData, data)
	})

	t.Run("handles connection errors", func(t *testing.T) {
		testData := []byte("test data")
		dataHash, err := multihash.Sum(testData, multihash.SHA2_256, -1)
		require.NoError(t, err)

		space := testutil.RandomIssuer(t)
		storageProvider := testutil.RandomPrincipal(t)

		// A URL that will fail to connect (default HTTP client, no in-process server).
		badURL := testutil.Must(url.Parse("http://localhost:99999"))(t)

		c := testutil.Must(ctestutil.Client(t))(t)

		proof := testutil.Must(contentcmds.Retrieve.Delegate(space, c.Issuer().DID(), space.DID()))(t)
		require.NoError(t, c.AddProofs(t.Context(), proof))

		location := locator.Location{
			Commitment: locator.Commitment{
				Node:     storageProvider.DID(),
				Space:    space.DID(),
				Content:  dataHash,
				Location: []commands.CborURL{commands.CborURL(*badURL)},
			},
			Range: blobindex.Range{Start: 0, End: int64(len(testData)) - 1},
		}

		_, err = c.Retrieve(t.Context(), location)
		require.Error(t, err)
	})
}
