package locator_test

import (
	"bytes"
	"context"
	"net/url"
	"slices"
	"testing"

	"github.com/fil-forge/indexing-service/pkg/types"
	"github.com/fil-forge/libforge/blobindex"
	"github.com/fil-forge/libforge/commands"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/digestutil"
	libtestutil "github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fil-forge/guppy/pkg/client/locator"
)

// --- helpers ---

// cborURLs parses url strings into the []commands.CborURL form stored on a
// location commitment.
func cborURLs(t *testing.T, strs ...string) []commands.CborURL {
	t.Helper()
	out := make([]commands.CborURL, 0, len(strs))
	for _, s := range strs {
		u := libtestutil.Must(url.Parse(s))(t)
		out = append(out, commands.CborURL(*u))
	}
	return out
}

// locationClaim builds an assert/location commitment invocation, as the indexer
// would return it.
func locationClaim(t *testing.T, provider ed25519.Signer, space did.DID, content mh.Multihash, urls ...string) ucan.Invocation {
	t.Helper()
	return libtestutil.Must(assertcmds.Location.Invoke(
		provider,
		provider.DID(),
		&assertcmds.LocationArguments{
			Space:    space,
			Content:  content,
			Location: cborURLs(t, urls...),
		},
	))(t)
}

// randomDelegation returns an arbitrary self-issued delegation, used as the
// retrieval authorization handed to the indexer.
func randomDelegation(t *testing.T) ucan.Delegation {
	t.Helper()
	s := libtestutil.RandomSigner(t)
	return libtestutil.Must(uploadcmds.Add.Delegate(s, s.DID(), s.DID()))(t)
}

// archiveIndex CAR-archives an index and returns its block CID and bytes.
func archiveIndex(t *testing.T, index blobindex.ShardedDagIndex) types.Block {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, blobindex.Archive(index, &buf))
	data := buf.Bytes()
	hash, err := mh.Sum(data, mh.SHA2_256, -1)
	require.NoError(t, err)
	return types.Block{Link: cid.NewCidV1(uint64(multicodec.Car), hash), Data: data}
}

func TestLocator(t *testing.T) {
	t.Run("locates block from indexer", func(t *testing.T) {
		blockHash := libtestutil.RandomMultihash(t)
		shardHash := libtestutil.RandomMultihash(t)

		space := libtestutil.RandomSigner(t)
		provider1 := libtestutil.RandomSigner(t)
		provider2 := libtestutil.RandomSigner(t)

		claim1 := locationClaim(t, provider1, space.DID(), shardHash,
			"https://storage1.example.com/block/abc123",
			"https://storage2.example.com/block/abc123",
		)
		claim2 := locationClaim(t, provider2, space.DID(), shardHash,
			"https://storage3.example.com/block/abc123",
		)

		index := blobindex.NewShardedDagIndex(-1)
		index.SetSlice(shardHash, blockHash, blobindex.Range{Start: 10, End: 2057})

		authDelegation := randomDelegation(t)

		mockIndexer := newMockIndexerClient(t, func(digests []mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error) {
			assert.ElementsMatch(t, []mh.Multihash{blockHash}, digests)
			return []ucan.Invocation{claim1, claim2}, []blobindex.ShardedDagIndex{index}, nil
		})
		l := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{authDelegation}, nil
		})

		locations, err := l.Locate(t.Context(), []did.DID{space.DID()}, blockHash)
		require.NoError(t, err)

		require.Len(t, locations, 2)
		require.ElementsMatch(t, cborURLs(t,
			"https://storage1.example.com/block/abc123",
			"https://storage2.example.com/block/abc123",
		), locations[0].Commitment.Location)
		require.Equal(t, blobindex.Range{Start: 10, End: 2057}, locations[0].Range)
		require.ElementsMatch(t, cborURLs(t,
			"https://storage3.example.com/block/abc123",
		), locations[1].Commitment.Location)
		require.Equal(t, blobindex.Range{Start: 10, End: 2057}, locations[1].Range)

		require.Len(t, mockIndexer.Queries, 1)
		require.Equal(t, types.Query{
			Hashes:      []mh.Multihash{blockHash},
			Delegations: []ucan.Delegation{authDelegation},
			Match:       types.Match{Subject: []did.DID{space.DID()}},
		}, mockIndexer.Queries[0])

		_, err = l.Locate(t.Context(), []did.DID{space.DID()}, blockHash)
		require.NoError(t, err)
		require.Len(t, mockIndexer.Queries, 1)
	})

	t.Run("caches unrequested blocks", func(t *testing.T) {
		block1Hash := libtestutil.RandomMultihash(t)
		block2Hash := libtestutil.RandomMultihash(t)
		shardHash := libtestutil.RandomMultihash(t)

		space := libtestutil.RandomSigner(t)
		provider := libtestutil.RandomSigner(t)

		claim := locationClaim(t, provider, space.DID(), shardHash, "https://storage.example.com/shard/xyz")

		index := blobindex.NewShardedDagIndex(-1)
		index.SetSlice(shardHash, block1Hash, blobindex.Range{Start: 0, End: 1023})
		index.SetSlice(shardHash, block2Hash, blobindex.Range{Start: 1024, End: 3071})

		auth := randomDelegation(t)
		mockIndexer := newMockIndexerClient(t, func(digests []mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error) {
			assert.ElementsMatch(t, []mh.Multihash{block1Hash}, digests)
			return []ucan.Invocation{claim}, []blobindex.ShardedDagIndex{index}, nil
		})
		l := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{auth}, nil
		})

		locations1, err := l.Locate(t.Context(), []did.DID{space.DID()}, block1Hash)
		require.NoError(t, err)
		require.Len(t, locations1, 1)
		require.Equal(t, blobindex.Range{Start: 0, End: 1023}, locations1[0].Range)
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard/xyz"), locations1[0].Commitment.Location)
		require.Len(t, mockIndexer.Queries, 1)

		locations2, err := l.Locate(t.Context(), []did.DID{space.DID()}, block2Hash)
		require.NoError(t, err)
		require.Len(t, locations2, 1)
		require.Equal(t, blobindex.Range{Start: 1024, End: 3071}, locations2[0].Range)
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard/xyz"), locations2[0].Commitment.Location)
		require.Len(t, mockIndexer.Queries, 1, "Second block should be served from cache")
	})

	t.Run("location-only query for block in cached index", func(t *testing.T) {
		block1Hash := libtestutil.RandomMultihash(t)
		block2Hash := libtestutil.RandomMultihash(t)
		shard1Hash := libtestutil.RandomMultihash(t)
		shard2Hash := libtestutil.RandomMultihash(t)

		space := libtestutil.RandomSigner(t)
		provider := libtestutil.RandomSigner(t)

		claim1 := locationClaim(t, provider, space.DID(), shard1Hash, "https://storage.example.com/shard1")

		index := blobindex.NewShardedDagIndex(-1)
		index.SetSlice(shard1Hash, block1Hash, blobindex.Range{Start: 0, End: 1023})
		index.SetSlice(shard2Hash, block2Hash, blobindex.Range{Start: 0, End: 2047})

		auth := randomDelegation(t)
		mockIndexer := newLocationQueryMockIndexer(t, claim1, index)
		l := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{auth}, nil
		})

		locations1, err := l.Locate(t.Context(), []did.DID{space.DID()}, block1Hash)
		require.NoError(t, err)
		require.Len(t, locations1, 1)
		require.Equal(t, blobindex.Range{Start: 0, End: 1023}, locations1[0].Range)
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard1"), locations1[0].Commitment.Location)

		require.Len(t, mockIndexer.Queries, 1)
		require.Equal(t, types.QueryTypeStandard, mockIndexer.Queries[0].Type)

		_, err = l.Locate(t.Context(), []did.DID{space.DID()}, block2Hash)
		require.Error(t, err)
		require.Contains(t, err.Error(), digestutil.Format(block2Hash))
		require.Contains(t, err.Error(), "no locations found")

		require.Len(t, mockIndexer.Queries, 2, "Should have made two queries")
		require.Equal(t, shard2Hash, mockIndexer.Queries[1].Hashes[0])
		require.Equal(t, types.QueryTypeLocation, mockIndexer.Queries[1].Type)
	})

	t.Run("cache is space-scoped", func(t *testing.T) {
		blockHash := libtestutil.RandomMultihash(t)
		shardHash := libtestutil.RandomMultihash(t)

		space1 := libtestutil.RandomSigner(t)
		space2 := libtestutil.RandomSigner(t)
		provider := libtestutil.RandomSigner(t)

		claim1 := locationClaim(t, provider, space1.DID(), shardHash, "https://storage1.example.com/space1/block")
		claim2 := locationClaim(t, provider, space2.DID(), shardHash, "https://storage2.example.com/space2/block")

		index := blobindex.NewShardedDagIndex(-1)
		index.SetSlice(shardHash, blockHash, blobindex.Range{Start: 0, End: 1023})

		auth := randomDelegation(t)
		mockIndexer := &spaceScopedMockIndexer{t: t, space1Claim: claim1, space2Claim: claim2, index: index}
		session := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{auth}, nil
		})

		locations1, err := session.Locate(t.Context(), []did.DID{space1.DID()}, blockHash)
		require.NoError(t, err)
		require.Len(t, locations1, 1)
		require.ElementsMatch(t, cborURLs(t, "https://storage1.example.com/space1/block"), locations1[0].Commitment.Location)

		locations2, err := session.Locate(t.Context(), []did.DID{space2.DID()}, blockHash)
		require.NoError(t, err)
		require.Len(t, locations2, 1)
		require.ElementsMatch(t, cborURLs(t, "https://storage2.example.com/space2/block"), locations2[0].Commitment.Location)

		require.Equal(t, 2, mockIndexer.queryCount, "Cache should be space-scoped")
	})

	t.Run("supports multi space locating", func(t *testing.T) {
		blockDigest1 := libtestutil.RandomMultihash(t)
		blockDigest2 := libtestutil.RandomMultihash(t)
		shardDigest1 := libtestutil.RandomMultihash(t)
		shardDigest2 := libtestutil.RandomMultihash(t)

		space1 := libtestutil.RandomSigner(t)
		space2 := libtestutil.RandomSigner(t)
		provider1 := libtestutil.RandomSigner(t)
		provider2 := libtestutil.RandomSigner(t)

		claim1 := locationClaim(t, provider1, space1.DID(), shardDigest1,
			"https://storage1.example.com/block/abc123",
			"https://storage2.example.com/block/abc123",
		)
		claim2 := locationClaim(t, provider2, space2.DID(), shardDigest2,
			"https://storage3.example.com/block/abc123",
		)

		index1 := blobindex.NewShardedDagIndex(-1)
		index1.SetSlice(shardDigest1, blockDigest1, blobindex.Range{Start: 10, End: 2057})
		index2 := blobindex.NewShardedDagIndex(-1)
		index2.SetSlice(shardDigest2, blockDigest2, blobindex.Range{Start: 57, End: 1080})

		auth := randomDelegation(t)
		required := []ucan.Delegation{auth}
		mockIndexer := newMockIndexerClient(t, func([]mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error) {
			return []ucan.Invocation{claim1, claim2}, []blobindex.ShardedDagIndex{index1, index2}, nil
		})
		l := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{auth}, nil
		})

		locations, err := l.Locate(t.Context(), []did.DID{space1.DID(), space2.DID()}, blockDigest1)
		require.NoError(t, err)
		require.Len(t, locations, 1)
		require.ElementsMatch(t, cborURLs(t,
			"https://storage1.example.com/block/abc123",
			"https://storage2.example.com/block/abc123",
		), locations[0].Commitment.Location)
		require.Equal(t, blobindex.Range{Start: 10, End: 2057}, locations[0].Range)

		require.Len(t, mockIndexer.Queries, 1)
		require.Equal(t, types.Query{
			Hashes:      []mh.Multihash{blockDigest1},
			Delegations: required,
			Match:       types.Match{Subject: []did.DID{space1.DID(), space2.DID()}},
		}, mockIndexer.Queries[0])

		locations, err = l.Locate(t.Context(), []did.DID{space1.DID(), space2.DID()}, blockDigest2)
		require.NoError(t, err)
		require.Len(t, locations, 1)
		require.ElementsMatch(t, cborURLs(t,
			"https://storage3.example.com/block/abc123",
		), locations[0].Commitment.Location)
		require.Equal(t, blobindex.Range{Start: 57, End: 1080}, locations[0].Range)

		require.Len(t, mockIndexer.Queries, 2)
		require.Equal(t, types.Query{
			Hashes:      []mh.Multihash{blockDigest2},
			Delegations: required,
			Match:       types.Match{Subject: []did.DID{space1.DID(), space2.DID()}},
		}, mockIndexer.Queries[1])
	})
}

func TestLocateMany(t *testing.T) {
	t.Run("locates multiple digests at once across two shards", func(t *testing.T) {
		block1Hash := libtestutil.RandomMultihash(t)
		block2Hash := libtestutil.RandomMultihash(t)
		block3Hash := libtestutil.RandomMultihash(t)
		shard1Hash := libtestutil.RandomMultihash(t)
		shard2Hash := libtestutil.RandomMultihash(t)

		space := libtestutil.RandomSigner(t)
		provider := libtestutil.RandomSigner(t)

		claim1 := locationClaim(t, provider, space.DID(), shard1Hash, "https://storage.example.com/shard1/abc")
		claim2 := locationClaim(t, provider, space.DID(), shard2Hash, "https://storage.example.com/shard2/def")

		index := blobindex.NewShardedDagIndex(-1)
		index.SetSlice(shard1Hash, block1Hash, blobindex.Range{Start: 0, End: 1023})
		index.SetSlice(shard1Hash, block2Hash, blobindex.Range{Start: 1024, End: 3071})
		index.SetSlice(shard2Hash, block3Hash, blobindex.Range{Start: 0, End: 511})

		auth := randomDelegation(t)
		mockIndexer := newMockIndexerClient(t, func(digests []mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error) {
			assert.ElementsMatch(t, []mh.Multihash{block1Hash, block2Hash, block3Hash}, digests)
			return []ucan.Invocation{claim1, claim2}, []blobindex.ShardedDagIndex{index}, nil
		})
		l := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{auth}, nil
		})

		locations, err := l.LocateMany(t.Context(), []did.DID{space.DID()}, []mh.Multihash{block1Hash, block2Hash, block3Hash})
		require.NoError(t, err)

		require.True(t, locations.Has(block1Hash))
		require.True(t, locations.Has(block2Hash))
		require.True(t, locations.Has(block3Hash))

		block1Locations := locations.Get(block1Hash)
		require.Len(t, block1Locations, 1)
		require.Equal(t, blobindex.Range{Start: 0, End: 1023}, block1Locations[0].Range)
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard1/abc"), block1Locations[0].Commitment.Location)

		block2Locations := locations.Get(block2Hash)
		require.Len(t, block2Locations, 1)
		require.Equal(t, blobindex.Range{Start: 1024, End: 3071}, block2Locations[0].Range)
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard1/abc"), block2Locations[0].Commitment.Location)

		block3Locations := locations.Get(block3Hash)
		require.Len(t, block3Locations, 1)
		require.Equal(t, blobindex.Range{Start: 0, End: 511}, block3Locations[0].Range)
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard2/def"), block3Locations[0].Commitment.Location)

		require.Len(t, mockIndexer.Queries, 1)
		require.ElementsMatch(t, []mh.Multihash{block1Hash, block2Hash, block3Hash}, mockIndexer.Queries[0].Hashes)
	})

	t.Run("returns partial results when some blocks not found", func(t *testing.T) {
		block1Hash := libtestutil.RandomMultihash(t)
		block2Hash := libtestutil.RandomMultihash(t)
		shardHash := libtestutil.RandomMultihash(t)

		space := libtestutil.RandomSigner(t)
		provider := libtestutil.RandomSigner(t)

		claim := locationClaim(t, provider, space.DID(), shardHash, "https://storage.example.com/shard/abc")

		index := blobindex.NewShardedDagIndex(-1)
		index.SetSlice(shardHash, block1Hash, blobindex.Range{Start: 0, End: 1023})

		auth := randomDelegation(t)
		mockIndexer := newMockIndexerClient(t, func(digests []mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error) {
			assert.ElementsMatch(t, []mh.Multihash{block1Hash, block2Hash}, digests)
			return []ucan.Invocation{claim}, []blobindex.ShardedDagIndex{index}, nil
		})
		l := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{auth}, nil
		})

		locations, err := l.LocateMany(t.Context(), []did.DID{space.DID()}, []mh.Multihash{block1Hash, block2Hash})
		require.NoError(t, err)

		require.True(t, locations.Has(block1Hash))
		require.False(t, locations.Has(block2Hash))

		block1Locations := locations.Get(block1Hash)
		require.Len(t, block1Locations, 1)
		require.Equal(t, blobindex.Range{Start: 0, End: 1023}, block1Locations[0].Range)
	})

	t.Run("batches queries for uncached blocks only", func(t *testing.T) {
		block1Hash := libtestutil.RandomMultihash(t)
		block2Hash := libtestutil.RandomMultihash(t)
		block3Hash := libtestutil.RandomMultihash(t)
		shard1Hash := libtestutil.RandomMultihash(t)
		shard2Hash := libtestutil.RandomMultihash(t)

		space := libtestutil.RandomSigner(t)
		provider := libtestutil.RandomSigner(t)

		claim1 := locationClaim(t, provider, space.DID(), shard1Hash, "https://storage.example.com/shard1/abc")
		claim2 := locationClaim(t, provider, space.DID(), shard2Hash, "https://storage.example.com/shard2/def")

		index := blobindex.NewShardedDagIndex(-1)
		index.SetSlice(shard1Hash, block1Hash, blobindex.Range{Start: 0, End: 1023})
		index.SetSlice(shard1Hash, block2Hash, blobindex.Range{Start: 1024, End: 3071})
		index.SetSlice(shard2Hash, block3Hash, blobindex.Range{Start: 0, End: 511})

		auth := randomDelegation(t)
		mockIndexer := newMockIndexerClient(t, func(digests []mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error) {
			return []ucan.Invocation{claim1, claim2}, []blobindex.ShardedDagIndex{index}, nil
		})
		l := locator.NewIndexLocator(mockIndexer, func(ctx context.Context, spaces []did.DID) ([]ucan.Delegation, error) {
			return []ucan.Delegation{auth}, nil
		})

		_, err := l.Locate(t.Context(), []did.DID{space.DID()}, block1Hash)
		require.NoError(t, err)
		require.Len(t, mockIndexer.Queries, 1)

		locations, err := l.LocateMany(t.Context(), []did.DID{space.DID()}, []mh.Multihash{block1Hash, block2Hash})
		require.NoError(t, err)
		require.True(t, locations.Has(block1Hash))
		require.True(t, locations.Has(block2Hash))
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard1/abc"), locations.Get(block1Hash)[0].Commitment.Location)
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard1/abc"), locations.Get(block2Hash)[0].Commitment.Location)
		require.Len(t, mockIndexer.Queries, 1, "All blocks should be served from cache")

		locations, err = l.LocateMany(t.Context(), []did.DID{space.DID()}, []mh.Multihash{block1Hash, block2Hash, block3Hash})
		require.NoError(t, err)
		require.True(t, locations.Has(block1Hash))
		require.True(t, locations.Has(block2Hash))
		require.True(t, locations.Has(block3Hash))
		require.ElementsMatch(t, cborURLs(t, "https://storage.example.com/shard2/def"), locations.Get(block3Hash)[0].Commitment.Location)
		require.Len(t, mockIndexer.Queries, 2, "block3 was not cached")

		locations, err = l.LocateMany(t.Context(), []did.DID{space.DID()}, []mh.Multihash{block1Hash, block2Hash, block3Hash})
		require.NoError(t, err)
		require.True(t, locations.Has(block1Hash))
		require.True(t, locations.Has(block2Hash))
		require.True(t, locations.Has(block3Hash))
		require.Len(t, mockIndexer.Queries, 2, "All blocks should be served from cache")
	})
}

// --- mock indexers ---

// claimSpaceContent decodes the space and content from a location commitment.
func claimSpaceContent(claim ucan.Invocation) (did.DID, mh.Multihash, bool) {
	if claim.Command() != assertcmds.Location.Command {
		return did.Undef, nil, false
	}
	var args assertcmds.LocationArguments
	if err := args.UnmarshalCBOR(bytes.NewReader(claim.ArgumentsBytes())); err != nil {
		return did.Undef, nil, false
	}
	return args.Space, args.Content, true
}

func newQueryResult(t *testing.T, claims []ucan.Invocation, indexBlocks []types.Block) *mockQueryResult {
	t.Helper()
	r := &mockQueryResult{}
	for _, claim := range claims {
		data := libtestutil.Must(invocation.Encode(claim))(t)
		r.blocks = append(r.blocks, types.Block{Link: claim.Link(), Data: data})
		r.claimLinks = append(r.claimLinks, claim.Link())
	}
	r.blocks = append(r.blocks, indexBlocks...)
	for _, b := range indexBlocks {
		r.indexLinks = append(r.indexLinks, b.Link)
	}
	return r
}

type mockQueryResult struct {
	blocks     []types.Block
	claimLinks []cid.Cid
	indexLinks []cid.Cid
}

var _ types.QueryResult = (*mockQueryResult)(nil)

func (m *mockQueryResult) Root() types.Block     { return types.Block{} }
func (m *mockQueryResult) Blocks() []types.Block { return m.blocks }
func (m *mockQueryResult) Claims() []cid.Cid     { return m.claimLinks }
func (m *mockQueryResult) Indexes() []cid.Cid    { return m.indexLinks }

// spaceScopedMockIndexer returns different claims based on the space in the query.
type spaceScopedMockIndexer struct {
	t           *testing.T
	space1Claim ucan.Invocation
	space2Claim ucan.Invocation
	index       blobindex.ShardedDagIndex
	queryCount  int
}

var _ locator.IndexerClient = (*spaceScopedMockIndexer)(nil)

func (m *spaceScopedMockIndexer) QueryClaims(ctx context.Context, query types.Query) (types.QueryResult, error) {
	m.queryCount++

	var claim ucan.Invocation
	if len(query.Match.Subject) > 0 {
		space1DID, _, _ := claimSpaceContent(m.space1Claim)
		if query.Match.Subject[0] == space1DID {
			claim = m.space1Claim
		} else {
			claim = m.space2Claim
		}
	}

	return newQueryResult(m.t, []ucan.Invocation{claim}, []types.Block{archiveIndex(m.t, m.index)}), nil
}

func newMockIndexerClient(t *testing.T, claimsAndIndexesFn func([]mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error)) *mockIndexerClient {
	return &mockIndexerClient{t: t, claimsAndIndexesFn: claimsAndIndexesFn}
}

type mockIndexerClient struct {
	t                  *testing.T
	claimsAndIndexesFn func([]mh.Multihash) ([]ucan.Invocation, []blobindex.ShardedDagIndex, error)

	Queries []types.Query
}

var _ locator.IndexerClient = (*mockIndexerClient)(nil)

func (m *mockIndexerClient) QueryClaims(ctx context.Context, query types.Query) (types.QueryResult, error) {
	m.Queries = append(m.Queries, query)

	allClaims, allIndexes, err := m.claimsAndIndexesFn(query.Hashes)
	if err != nil {
		return nil, err
	}

	indexes := []blobindex.ShardedDagIndex{}
	shards := []mh.Multihash{}
	for _, index := range allIndexes {
		for _, queryDigest := range query.Hashes {
			if shard, ok := indexContains(index, queryDigest); ok {
				if !slices.Contains(indexes, index) {
					indexes = append(indexes, index)
				}
				if !slices.ContainsFunc(shards, func(s mh.Multihash) bool { return s.String() == shard.String() }) {
					shards = append(shards, shard)
				}
			}
		}
	}

	var indexBlocks []types.Block
	for _, index := range indexes {
		indexBlocks = append(indexBlocks, archiveIndex(m.t, index))
	}

	claims := []ucan.Invocation{}
	for _, claim := range allClaims {
		space, content, ok := claimSpaceContent(claim)
		if !ok {
			continue
		}
		for _, shard := range shards {
			if content.String() == shard.String() && slices.Contains(query.Match.Subject, space) {
				claims = append(claims, claim)
				break
			}
		}
	}

	return newQueryResult(m.t, claims, indexBlocks), nil
}

func indexContains(index blobindex.ShardedDagIndex, digest mh.Multihash) (mh.Multihash, bool) {
	for shard, slices := range index.Shards().Iterator() {
		if shard.String() == digest.String() {
			return shard, true
		}
		for slice := range slices.Iterator() {
			if slice.String() == digest.String() {
				return shard, true
			}
		}
	}
	return nil, false
}

// locationQueryMockIndexer returns indexes on standard queries, but only claims
// on location queries.
type locationQueryMockIndexer struct {
	t     *testing.T
	claim ucan.Invocation
	index blobindex.ShardedDagIndex

	Queries []types.Query
}

var _ locator.IndexerClient = (*locationQueryMockIndexer)(nil)

func newLocationQueryMockIndexer(t *testing.T, claim ucan.Invocation, index blobindex.ShardedDagIndex) *locationQueryMockIndexer {
	return &locationQueryMockIndexer{t: t, claim: claim, index: index}
}

func (m *locationQueryMockIndexer) QueryClaims(ctx context.Context, query types.Query) (types.QueryResult, error) {
	m.Queries = append(m.Queries, query)

	// For location-only queries, return empty (simulating no location for shard2).
	if query.Type == types.QueryTypeLocation {
		return newQueryResult(m.t, nil, nil), nil
	}

	// For standard queries, return both index and claim.
	return newQueryResult(m.t, []ucan.Invocation{m.claim}, []types.Block{archiveIndex(m.t, m.index)}), nil
}
