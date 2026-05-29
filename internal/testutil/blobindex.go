// Package testutil provides guppy-local test helpers that fill gaps not covered
// by libforge's testutil package (e.g. ShardedDagIndex fixtures).
package testutil

import (
	"testing"

	"github.com/fil-forge/libforge/blobindex"
	libtestutil "github.com/fil-forge/libforge/testutil"
	mh "github.com/multiformats/go-multihash"
)

// RandomShardedDagIndexView returns a ShardedDagIndex containing `size` shards,
// each with a single random slice, along with the digest of the last slice
// added. It replaces go-libstoracha/testutil.RandomShardedDagIndexView, which
// has no libforge equivalent.
func RandomShardedDagIndexView(t *testing.T, size int) (mh.Multihash, blobindex.ShardedDagIndex) {
	t.Helper()
	index := blobindex.NewShardedDagIndex(size)
	var lastSlice mh.Multihash
	for range size {
		shard := libtestutil.RandomMultihash(t)
		slice := libtestutil.RandomMultihash(t)
		index.SetSlice(shard, slice, blobindex.Range{Start: 0, End: 99})
		lastSlice = slice
	}
	return lastSlice, index
}
