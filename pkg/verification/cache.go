package verification

import (
	"bytes"
	"maps"
	"slices"
	"sync"

	"github.com/fil-forge/libforge/blobindex"
	"github.com/fil-forge/libforge/bytemap"
	"github.com/fil-forge/libforge/commands/assert"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

type IndexCache struct {
	indexes    map[cid.Cid]blobindex.ShardedDagIndex
	sliceIndex bytemap.ByteMap[multihash.Multihash, cid.Cid]
	mutex      sync.RWMutex
}

func NewIndexCache() *IndexCache {
	return &IndexCache{
		indexes:    map[cid.Cid]blobindex.ShardedDagIndex{},
		sliceIndex: bytemap.NewByteMap[multihash.Multihash, cid.Cid](0),
	}
}

func (c *IndexCache) Add(root cid.Cid, index blobindex.ShardedDagIndex) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.indexes[root] = index
	for _, slices := range index.Shards().Iterator() {
		for slice := range slices.Iterator() {
			c.sliceIndex.Set(slice, root)
		}
	}
}

func (c *IndexCache) IndexForSlice(slice multihash.Multihash) (blobindex.ShardedDagIndex, bool) {
	root := c.sliceIndex.Get(slice)
	idx, ok := c.indexes[root]
	return idx, ok
}

type Location struct {
	Commitment ucan.Invocation
	// Arguments are the decoded caveats from the commitment delegation.
	Arguments *assert.LocationArguments
}

type LocationCache struct {
	// shard digest -> commitment CID -> location info
	locations bytemap.ByteMap[multihash.Multihash, map[cid.Cid]Location]
	mutex     sync.RWMutex
}

func NewLocationCache() *LocationCache {
	return &LocationCache{
		locations: bytemap.NewByteMap[multihash.Multihash, map[cid.Cid]Location](0),
	}
}

func (c *LocationCache) Add(commitment ucan.Invocation) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var args assert.LocationArguments
	if err := args.UnmarshalCBOR(bytes.NewReader(commitment.ArgumentsBytes())); err != nil {
		log.Warnw("adding location to cache", "error", err)
		return
	}

	shardLocations := c.locations.Get(args.Content)
	if shardLocations == nil {
		shardLocations = map[cid.Cid]Location{}
		c.locations.Set(args.Content, shardLocations)
	}
	shardLocations[commitment.Link()] = Location{Commitment: commitment, Arguments: &args}
}

func (c *LocationCache) LocationsForShard(shard multihash.Multihash) []Location {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return slices.Collect(maps.Values(c.locations.Get(shard)))
}
