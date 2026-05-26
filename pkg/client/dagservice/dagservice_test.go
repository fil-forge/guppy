package dagservice_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/fil-forge/guppy/pkg/client/dagservice"
	"github.com/fil-forge/guppy/pkg/client/locator"
	"github.com/fil-forge/libforge/blobindex"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/did"
	dag "github.com/ipfs/boxo/ipld/merkledag"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

func TestDAGService(t *testing.T) {
	space := testutil.RandomDID(t)

	t.Run("retrieves a block via the DAG service", func(t *testing.T) {

		testCases := []struct {
			name      string
			codec     uint64
			blockData []byte
		}{
			{
				name:  "DAG-PB node",
				codec: cid.DagProtobuf,
				blockData: func() []byte {
					// Create a simple DAG-PB node
					node := dag.NodeWithData(testutil.RandomBytes(t, 64))
					data, err := node.Marshal()
					require.NoError(t, err)
					return data
				}(),
			},
			{
				name:      "raw node",
				codec:     cid.Raw,
				blockData: testutil.RandomBytes(t, 256),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// The block data should be embedded in a larger shard
				shardData := testutil.RandomBytes(t, 1024)
				offset := uint64(512)
				length := uint64(len(tc.blockData))
				copy(shardData[offset:offset+length], tc.blockData)

				// Create a CID from the block data
				hash, err := mh.Sum(tc.blockData, mh.SHA2_256, -1)
				require.NoError(t, err)
				blockCID := cid.NewCidV1(tc.codec, hash)

				location := locator.Location{
					Commitment: locator.Commitment{
						Space:   space,
						Content: hash,
					},
					Range: blobindex.Range{
						Start: int64(offset),
						End:   int64(offset+length) - 1,
					},
				}

				lctr := newStubLocator()
				lctr.locations.Set(blockCID.Hash(), []locator.Location{location})

				retriever := newMockRetriever(map[string][]byte{
					string(hash): shardData,
				})

				ds := dagservice.NewDAGService(
					lctr,
					retriever,
					[]did.DID{space},
				)

				node, err := ds.Get(t.Context(), blockCID)
				require.NoError(t, err)
				require.Equal(t, blockCID, node.Cid())
			})
		}
	})

	t.Run("retrieves multiple, contiguous blocks via the DAG service", func(t *testing.T) {
		shardData := []byte("here is some slice data, and more slice data")
		shardCID := cidFor(shardData)

		block1Offset := uint64(8)
		block1Length := uint64(15) // "some slice data"
		block1Data := shardData[block1Offset : block1Offset+block1Length]
		block2Offset := uint64(23)
		block2Length := uint64(15) // ", and more slice data"
		block2Data := shardData[block2Offset : block2Offset+block2Length]

		block1CID := cidFor(block1Data)
		block2CID := cidFor(block2Data)

		shardCommitment := locator.Commitment{
			Space:   space,
			Content: shardCID.Hash(),
		}

		location1 := locator.Location{
			Commitment: shardCommitment,
			Range: blobindex.Range{
				Start: int64(block1Offset),
				End:   int64(block1Offset+block1Length) - 1,
			},
		}

		location2 := locator.Location{
			Commitment: shardCommitment,
			Range: blobindex.Range{
				Start: int64(block2Offset),
				End:   int64(block2Offset+block2Length) - 1,
			},
		}

		lctr := newStubLocator()
		lctr.locations.Set(block1CID.Hash(), []locator.Location{location1})
		lctr.locations.Set(block2CID.Hash(), []locator.Location{location2})

		retriever := newMockRetriever(map[string][]byte{
			string(shardCID.Hash()): shardData,
		})

		ds := dagservice.NewDAGService(
			lctr,
			retriever,
			[]did.DID{space},
		)

		nodesCh := ds.GetMany(t.Context(), []cid.Cid{block1CID, block2CID})
		var (
			node1 blocks.Block
			node2 blocks.Block
		)
		for node := range nodesCh {
			require.NoError(t, node.Err)
			if node.Node.Cid() == block1CID {
				node1 = node.Node
			} else if node.Node.Cid() == block2CID {
				node2 = node.Node
			} else {
				require.FailNowf(t, "unexpected block CID: %s", node.Node.Cid().String())
			}
		}

		require.NotNil(t, node1)
		require.Equal(t, block1Data, node1.RawData())
		require.NotNil(t, node2)
		require.Equal(t, block2Data, node2.RawData())

		require.Len(t, retriever.requests, 1)
	})
}

func cidFor(data []byte) cid.Cid {
	hash, err := mh.Sum(data, mh.SHA2_256, -1)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(cid.Raw, hash)
}

func newStubLocator() stubLocator {
	return stubLocator{
		locations: blobindex.NewMultihashMap[[]locator.Location](-1),
	}
}

type stubLocator struct {
	locations blobindex.MultihashMap[[]locator.Location]
}

var _ locator.Locator = stubLocator{}

func (m stubLocator) Locate(ctx context.Context, spaces []did.DID, digest mh.Multihash) ([]locator.Location, error) {
	if m.locations.Has(digest) {
		return m.locations.Get(digest), nil
	}
	return nil, nil
}

func (m stubLocator) LocateMany(ctx context.Context, spaces []did.DID, digests []mh.Multihash) (blobindex.MultihashMap[[]locator.Location], error) {
	result := blobindex.NewMultihashMap[[]locator.Location](len(digests))
	for _, digest := range digests {
		if m.locations.Has(digest) {
			result.Set(digest, m.locations.Get(digest))
		}
	}
	return result, nil
}

// newMockRetriever builds a retriever whose responses are keyed by the string
// form of a location commitment's Content digest.
func newMockRetriever(responses map[string][]byte) *mockRetriever {
	return &mockRetriever{data: responses}
}

type mockRetriever struct {
	data     map[string][]byte
	requests []locator.Location
	mu       sync.Mutex
}

var _ dagservice.Retriever = (*mockRetriever)(nil)

func (r *mockRetriever) Retrieve(ctx context.Context, location locator.Location) (io.ReadCloser, error) {
	data, ok := r.data[string(location.Commitment.Content)]
	if !ok {
		return nil, fmt.Errorf("no data for content %x", location.Commitment.Content)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, location)
	return io.NopCloser(bytes.NewReader(data[location.Range.Start : location.Range.End+1])), nil
}
