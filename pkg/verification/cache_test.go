package verification_test

import (
	"net/url"
	"testing"

	gtestutil "github.com/fil-forge/guppy/internal/testutil"
	"github.com/fil-forge/guppy/pkg/verification"
	"github.com/fil-forge/libforge/blobindex"
	"github.com/fil-forge/libforge/commands"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

func TestIndexCache(t *testing.T) {
	t.Run("returns false when looking up a slice that was never added", func(t *testing.T) {
		cache := verification.NewIndexCache()

		unknownSlice := testutil.RandomMultihash(t)
		_, found := cache.IndexForSlice(unknownSlice)

		require.False(t, found, "should not find an index for unknown slice")
	})

	t.Run("returns the index that contains a given slice", func(t *testing.T) {
		cache := verification.NewIndexCache()

		rootCID := testutil.RandomCID(t)
		index, sliceDigest := randomIndex(t)

		cache.Add(rootCID, index)

		foundIndex, found := cache.IndexForSlice(sliceDigest)

		require.True(t, found, "should find index for known slice")
		require.Equal(t, index, foundIndex, "found index should match the one that was added")
	})

	t.Run("handles multiple indexes with different slices", func(t *testing.T) {
		cache := verification.NewIndexCache()

		root1 := testutil.RandomCID(t)
		index1, slice1 := randomIndex(t)

		root2 := testutil.RandomCID(t)
		index2, slice2 := randomIndex(t)

		cache.Add(root1, index1)
		cache.Add(root2, index2)

		foundIndex1, found1 := cache.IndexForSlice(slice1)
		require.True(t, found1)
		require.Equal(t, index1, foundIndex1)

		foundIndex2, found2 := cache.IndexForSlice(slice2)
		require.True(t, found2)
		require.Equal(t, index2, foundIndex2)
	})
}

func TestLocationCache(t *testing.T) {
	t.Run("returns empty slice when looking up an unknown shard", func(t *testing.T) {
		cache := verification.NewLocationCache()

		unknownShard := testutil.RandomMultihash(t)
		locations := cache.LocationsForShard(unknownShard)

		require.Empty(t, locations, "should return no locations for unknown shard")
	})

	t.Run("stores and retrieves a location commitment for a shard", func(t *testing.T) {
		cache := verification.NewLocationCache()

		// Create a location commitment
		issuer := testutil.Must(ed25519.Generate())(t)
		audience := testutil.Must(ed25519.Generate())(t)
		space := testutil.Must(ed25519.Generate())(t)

		shardDigest := testutil.RandomMultihash(t)
		commitment := createLocationCommitment(t, issuer, audience, space.DID(), shardDigest)

		cache.Add(commitment)

		// Should be able to find the location by shard digest
		locations := cache.LocationsForShard(shardDigest)

		require.Len(t, locations, 1, "should have one location for the shard")
		require.Equal(t, commitment.Link(), locations[0].Commitment.Link())
		require.Equal(t, space.DID(), locations[0].Arguments.Space)
	})

	t.Run("stores multiple locations for the same shard from different providers", func(t *testing.T) {
		cache := verification.NewLocationCache()

		// Two different storage providers (issuers) for the same shard
		issuer1 := testutil.Must(ed25519.Generate())(t)
		issuer2 := testutil.Must(ed25519.Generate())(t)
		audience := testutil.Must(ed25519.Generate())(t)
		space := testutil.Must(ed25519.Generate())(t)

		shardDigest := testutil.RandomMultihash(t)

		// Different storage providers make different commitments for the same shard
		commitment1 := createLocationCommitment(t, issuer1, audience, space.DID(), shardDigest)
		commitment2 := createLocationCommitment(t, issuer2, audience, space.DID(), shardDigest)

		cache.Add(commitment1)
		cache.Add(commitment2)

		locations := cache.LocationsForShard(shardDigest)

		require.Len(t, locations, 2, "should have two locations for the shard")
	})

	t.Run("deduplicates identical commitments for the same shard", func(t *testing.T) {
		cache := verification.NewLocationCache()

		issuer := testutil.Must(ed25519.Generate())(t)
		audience := testutil.Must(ed25519.Generate())(t)
		space := testutil.Must(ed25519.Generate())(t)

		shardDigest := testutil.RandomMultihash(t)

		// Adding the same commitment twice should not create duplicates
		commitment := createLocationCommitment(t, issuer, audience, space.DID(), shardDigest)

		cache.Add(commitment)
		cache.Add(commitment) // Add the same commitment again

		locations := cache.LocationsForShard(shardDigest)

		require.Len(t, locations, 1, "should have only one location (deduped)")
	})

	t.Run("keeps locations for different shards separate", func(t *testing.T) {
		cache := verification.NewLocationCache()

		issuer := testutil.Must(ed25519.Generate())(t)
		audience := testutil.Must(ed25519.Generate())(t)
		space := testutil.Must(ed25519.Generate())(t)

		shard1 := testutil.RandomMultihash(t)
		shard2 := testutil.RandomMultihash(t)

		commitment1 := createLocationCommitment(t, issuer, audience, space.DID(), shard1)
		commitment2 := createLocationCommitment(t, issuer, audience, space.DID(), shard2)

		cache.Add(commitment1)
		cache.Add(commitment2)

		locations1 := cache.LocationsForShard(shard1)
		locations2 := cache.LocationsForShard(shard2)

		require.Len(t, locations1, 1, "shard1 should have one location")
		require.Len(t, locations2, 1, "shard2 should have one location")
		require.NotEqual(t, locations1[0].Commitment.Link(), locations2[0].Commitment.Link())
	})
}

// randomIndex generates a ShardedDagIndex with a single slice and returns it,
// along with the digest of that slice.
func randomIndex(t *testing.T) (blobindex.ShardedDagIndex, multihash.Multihash) {
	t.Helper()
	slice, index := gtestutil.RandomShardedDagIndexView(t, 1)
	return index, slice
}

func createLocationCommitment(t *testing.T, issuer, audience ucan.Signer, space did.DID, shardDigest multihash.Multihash) ucan.Invocation {
	t.Helper()

	storageURL := testutil.Must(url.Parse("https://storage.example.com/blob/test"))(t)

	return testutil.Must(assertcmds.Location.Invoke(
		issuer,
		issuer.DID(),
		&assertcmds.LocationArguments{
			Space:    space,
			Content:  shardDigest,
			Location: []commands.CborURL{commands.CborURL(*storageURL)},
		},
		invocation.WithAudience(audience.DID()),
	))(t)
}
