package dagservice

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"math/rand"
	"slices"
	"sync"

	"github.com/fil-forge/guppy/pkg/client/locator"
	"github.com/fil-forge/libforge/blobindex"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/ipfs/boxo/exchange"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	mh "github.com/multiformats/go-multihash"
)

var log = logging.Logger("client/dagservice")

// DefaultMaxGap is the default maximum gap (in bytes) between blocks that can
// still be coalesced into a single request. This value accommodates the typical
// overhead between adjacent blocks in a CAR file, which consists of a varint
// length prefix (1-10 bytes) and a CID (34-37 bytes for CIDv0/CIDv1 with
// SHA2-256).
const DefaultMaxGap = 64

type storachaExchange struct {
	locator   locator.Locator
	retriever Retriever
	spaces    []did.DID
	shards    blobindex.MultihashMap[[]byte]
	maxGap    uint64
}

var _ exchange.Interface = (*storachaExchange)(nil)

// ExchangeOption configures a storachaExchange.
type ExchangeOption func(*storachaExchange)

// WithMaxGap sets the maximum gap (in bytes) between blocks that can still be
// coalesced into a single request. A maxGap of 0 means blocks must be exactly
// contiguous to be coalesced. The default is [DefaultMaxGap].
func WithMaxGap(maxGap uint64) ExchangeOption {
	return func(se *storachaExchange) {
		se.maxGap = maxGap
	}
}

// NewExchange creates a new exchange for retrieving blocks from Storacha.
func NewExchange(locator locator.Locator, retriever Retriever, spaces []did.DID, opts ...ExchangeOption) exchange.Interface {
	se := &storachaExchange{
		locator:   locator,
		retriever: retriever,
		spaces:    spaces,
		shards:    blobindex.NewMultihashMap[[]byte](-1),
		maxGap:    DefaultMaxGap,
	}
	for _, opt := range opts {
		opt(se)
	}
	return se
}

func (se *storachaExchange) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	log.Debugw("Getting block", "cid", c, "spaces", se.spaces)
	locations, err := se.locator.Locate(ctx, se.spaces, c.Hash())
	if err != nil {
		return nil, fmt.Errorf("locating block %s: %w", c, err)
	}

	// Randomly pick one of the available locations
	location := locations[rand.Intn(len(locations))]

	blockReader, err := se.retriever.Retrieve(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("retrieving block %s: %w", c, err)
	}
	defer blockReader.Close()

	blockBytes, err := io.ReadAll(blockReader)
	if err != nil {
		return nil, fmt.Errorf("reading block %s data: %w", c.String(), err)
	}

	block, err := makeBlock(blockBytes, c)
	if err != nil {
		return nil, fmt.Errorf("creating block %s (data length: %d): %w", c.String(), len(blockBytes), err)
	}

	return block, nil
}

// makeBlock creates a block from the given data and CID, verifying that the
// data matches the expected hash in the CID.
func makeBlock(data []byte, c cid.Cid) (blocks.Block, error) {
	expectedDigest := c.Hash()
	decHash, err := mh.Decode(expectedDigest)
	if err != nil {
		return nil, fmt.Errorf("decoding content multihash %s: %w", expectedDigest, err)
	}

	actualDigest, err := mh.Sum(data, decHash.Code, -1)
	if err != nil {
		return nil, fmt.Errorf("hashing content %s: %w", expectedDigest, err)
	}

	if !bytes.Equal(expectedDigest, actualDigest) {
		return nil, fmt.Errorf("content hash mismatch for content %s; got %s", digestutil.Format(expectedDigest), digestutil.Format(actualDigest))
	}

	block, err := blocks.NewBlockWithCid(data, c)
	if err != nil {
		return nil, fmt.Errorf("creating block %s (data length: %d): %w", c.String(), len(data), err)
	}

	return block, nil
}

type coalescedLocation struct {
	location *locator.Location
	slices   []slice
}

type slice struct {
	cid       cid.Cid
	byteRange blobindex.Range
}

func sameShard(a, b locator.Location) bool {
	return bytes.Equal(a.Commitment.Content, b.Commitment.Content)
}

// withinGap checks if location b is within maxGap bytes after the end of
// location a, in the same shard. A maxGap of 0 means they must be exactly
// contiguous.
func withinGap(a, b locator.Location, maxGap uint64) bool {
	if !sameShard(a, b) {
		return false
	}

	// Range.End is inclusive (the last byte index), so the number of bytes
	// strictly between a and b is (b.Start - a.End - 1). Exactly-contiguous
	// slices (b.Start == a.End+1) therefore have a gap of 0.
	if b.Range.Start > a.Range.End && b.Range.Start <= a.Range.End+int64(maxGap)+1 {
		return true
	} else {
		log.Debugf("Locations not within gap: a ends at %d, b starts at %d; gap is %d, maxGap is %d", a.Range.End, b.Range.Start, b.Range.Start-a.Range.End-1, maxGap)
		return false
	}
}

// GetBlocks will attempt to fetch multiple contiguous blocks in a single
// request where possible.
func (se *storachaExchange) GetBlocks(ctx context.Context, cids []cid.Cid) (<-chan blocks.Block, error) {
	log.Debugw("Getting blocks", "count", len(cids), "spaces", se.spaces)
	out := make(chan blocks.Block)

	digests := make([]mh.Multihash, 0, len(cids))
	for _, c := range cids {
		digests = append(digests, c.Hash())
	}

	locations, err := se.locator.LocateMany(ctx, se.spaces, digests)
	if err != nil {
		return nil, fmt.Errorf("locating blocks: %w", err)
	}

	// Sort the CIDs by the offset of their first location. This is a best effort
	// to ensure that contiguous blocks are adjacent in the list. It can fail if a
	// block is available in multiple shards, and thus at multiple offsets; the
	// offset we sort by may not be the one that allows coalescing with the
	// running batch of blocks when we get to it. But the most common case is that
	// a block is found in a single shard (which may be replicated to multiple
	// locations), so the first location's offset is a reasonable heuristic.
	slices.SortFunc(cids, func(cidA, cidB cid.Cid) int {
		locsA := locations.Get(cidA.Hash())
		locsB := locations.Get(cidB.Hash())

		if len(locsA) == 0 || len(locsB) == 0 {
			return 0
		}

		return cmp.Compare(locsA[0].Range.Start, locsB[0].Range.Start)
	})

	var coalescedLocations []coalescedLocation
	var currentLocation coalescedLocation
	for _, cid := range cids {
		locs := locations.Get(cid.Hash())
		if len(locs) == 0 {
			return nil, fmt.Errorf("no locations found for block %s", cid.String())
		}

		// If we don't have a current location, make one
		if currentLocation.location == nil {
			loc := locs[rand.Intn(len(locs))]
			currentLocation = coalescedLocation{
				location: &loc,
				slices: []slice{
					{
						cid: cid,
						byteRange: blobindex.Range{
							Start: loc.Range.Start,
							End:   loc.Range.End,
						},
					},
				},
			}
		} else {
			// See if any of the locations for this block are within the max gap of
			// the current location.
			found := false
			if currentLocation.location != nil {
				for _, loc := range locs {
					if withinGap(*currentLocation.location, loc, se.maxGap) {
						// Extend the coalesced location to include the gap and this block
						currentLocation.location.Range.End = loc.Range.End
						currentLocation.slices = append(currentLocation.slices, slice{
							cid: cid,
							byteRange: blobindex.Range{
								Start: loc.Range.Start,
								End:   loc.Range.End,
							},
						})
						found = true
						break
					}
				}
			}

			// If none of the locations for this block are contiguous with the
			// current location, emit, then make a new one.
			if !found {
				coalescedLocations = append(coalescedLocations, currentLocation)
				loc := locs[rand.Intn(len(locs))]
				currentLocation = coalescedLocation{
					location: &loc,
					slices: []slice{
						{
							cid: cid,
							byteRange: blobindex.Range{
								Start: loc.Range.Start,
								End:   loc.Range.End,
							},
						},
					},
				}
			}
		}
	}

	if currentLocation.location != nil {
		coalescedLocations = append(coalescedLocations, currentLocation)
	}

	var wg sync.WaitGroup
	for _, cloc := range coalescedLocations {
		log.Infof("Fetching %d coalesced blocks at offset %d with length %d", len(cloc.slices), cloc.location.Range.Start, cloc.location.Range.End-cloc.location.Range.Start+1)
		wg.Add(1)
		go func(cloc coalescedLocation) {
			defer wg.Done()
			blockReader, err := se.retriever.Retrieve(ctx, *cloc.location)
			if err != nil {
				log.Errorf("retrieving blocks starting at offset %d: %v", cloc.location.Range.Start, err)
				return
			}
			defer blockReader.Close()

			// Slices appear in order, and cannot overlap, so we can slice up the data
			// as we read it.
			pos := cloc.location.Range.Start
			for _, slice := range cloc.slices {
				// Skip to slice offset
				skipped, err := io.CopyN(io.Discard, blockReader, int64(slice.byteRange.Start-pos))
				pos += skipped
				if err != nil {
					log.Errorf("skipping to block %s at %d-%d: expected to skip %d bytes, skipped %d: %v", slice.cid.String(), slice.byteRange.Start, slice.byteRange.End, slice.byteRange.Start-pos, skipped, err)
					return
				}

				// Read slice data
				sliceBytes := make([]byte, slice.byteRange.End-slice.byteRange.Start+1)
				read, err := io.ReadFull(blockReader, sliceBytes)
				pos += int64(read)
				if err != nil {
					log.Errorf("reading block %s at %d-%d: expected %d bytes, got %d: %v", slice.cid.String(), slice.byteRange.Start, slice.byteRange.End, slice.byteRange.End-slice.byteRange.Start+1, read, err)
					return
				}

				// Create block
				blk, err := makeBlock(sliceBytes, slice.cid)
				if err != nil {
					log.Errorf("creating block %s at %d-%d: %v", slice.cid.String(), slice.byteRange.Start, slice.byteRange.End, err)
					return
				}

				out <- blk
			}
		}(cloc)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

func (se *storachaExchange) NotifyNewBlocks(ctx context.Context, blocks ...blocks.Block) error {
	return nil
}
func (se *storachaExchange) Close() error {
	return nil
}
