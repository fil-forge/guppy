package verification

import (
	"bytes"
	"context"
	"fmt"

	"github.com/cenkalti/backoff/v5"
	"github.com/fil-forge/indexing-service/pkg/types"
	"github.com/fil-forge/libforge/blobindex"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

// IndexingServiceClient is the interface for querying the indexing service.
type IndexingServiceClient interface {
	QueryClaims(ctx context.Context, query types.Query) (types.QueryResult, error)
}

type Indexer struct {
	client        IndexingServiceClient
	authorize     AuthorizeIndexerRetrievalFunc
	indexCache    *IndexCache
	locationCache *LocationCache
}

// NewIndexer creates a new Indexer with the given indexing service client and
// authorization function.
func NewIndexer(client IndexingServiceClient, authorize AuthorizeIndexerRetrievalFunc) *Indexer {
	return &Indexer{
		client:        client,
		authorize:     authorize,
		indexCache:    NewIndexCache(),
		locationCache: NewLocationCache(),
	}
}

// do a query and cache the results
func (i *Indexer) doQuery(ctx context.Context, digest multihash.Multihash) error {
	result, err := backoff.Retry(ctx, func() (types.QueryResult, error) {
		q := types.Query{Hashes: []multihash.Multihash{digest}}
		auth, err := i.authorize()
		if err != nil {
			return nil, fmt.Errorf("authorizing indexer retrieval: %w", err)
		}
		if len(auth) == 0 {
			return nil, fmt.Errorf("no authorization provided for indexer retrieval")
		}

		q.Match = types.Match{Subject: []did.DID{auth[0].Subject()}}
		q.Delegations = auth

		return i.client.QueryClaims(ctx, q)
	}, backoff.WithMaxTries(3))
	if err != nil {
		return fmt.Errorf("querying indexer for slice %s: %w", digestutil.Format(digest), err)
	}

	blocks := map[cid.Cid][]byte{}
	for _, b := range result.Blocks() {
		blocks[b.Link] = b.Data
	}

	for _, root := range result.Indexes() {
		indexBytes, ok := blocks[root]
		if !ok {
			log.Warnw("index block not found in response", "index", root)
			continue
		}
		index, err := blobindex.Extract(bytes.NewReader(indexBytes))
		if err != nil {
			return fmt.Errorf("extracting index: %w", err)
		}
		i.indexCache.Add(root, index)
	}

	for _, root := range result.Claims() {
		b, ok := blocks[root]
		if !ok {
			log.Warnw("claim block not found in response", "root", root)
			continue
		}
		claim, err := invocation.Decode(b)
		if err != nil {
			log.Warnw("failed to decode claim", "root", root, "error", err)
			continue
		}
		if claim.Command() != assertcmds.Location.Command {
			continue
		}
		i.locationCache.Add(claim)
	}

	return nil
}

// FindShard finds the shard and its position for the given slice.
func (i *Indexer) FindShard(ctx context.Context, slice multihash.Multihash) (multihash.Multihash, blobindex.Range, error) {
	index, ok := i.indexCache.IndexForSlice(slice)
	if !ok {
		err := i.doQuery(ctx, slice)
		if err != nil {
			return nil, blobindex.Range{}, err
		}
		idx, ok := i.indexCache.IndexForSlice(slice)
		if !ok {
			return nil, blobindex.Range{}, fmt.Errorf("index for slice not found: %s", digestutil.Format(slice))
		}
		index = idx
	}

	for shard, slices := range index.Shards().Iterator() {
		for s, pos := range slices.Iterator() {
			if bytes.Equal(s, slice) {
				return shard, pos, nil
			}
		}
	}
	return nil, blobindex.Range{}, fmt.Errorf("slice not found in index: %s", digestutil.Format(slice))
}

// FindLocations finds at least 1 location for the given shard or returns an
// error.
func (i *Indexer) FindLocations(ctx context.Context, shard multihash.Multihash) ([]Location, error) {
	locations := i.locationCache.LocationsForShard(shard)
	if len(locations) == 0 {
		err := i.doQuery(ctx, shard)
		if err != nil {
			return nil, err
		}
		locations = i.locationCache.LocationsForShard(shard)
		if len(locations) == 0 {
			return nil, fmt.Errorf("location for shard %q not found", digestutil.Format(shard))
		}
	}
	return locations, nil
}
