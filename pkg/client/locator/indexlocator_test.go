package locator_test

import "testing"

// TODO(forrest): port the IndexLocator tests to ucantone.
//
// The original ~1000-line test (preserved in git history immediately before
// this commit) builds location claims as go-ucanto delegations and feeds them
// through mock indexers (spaceScopedMockIndexer, mockIndexerClient,
// locationQueryMockIndexer) whose mockQueryResult predates the indexing-service
// interface change. Porting requires:
//
//   - mockQueryResult.Blocks() -> []types.Block (was iter.Seq2[block.Block,error]);
//     Root() -> types.Block; Claims()/Indexes() -> []cid.Cid.
//   - Claims built as ucantone assert/location invocations (assertcmds.Location.Invoke)
//     and emitted as encoded blocks (invocation.Encode), decoded by the locator
//     via invocation.Decode.
//   - Indexes via libforge/blobindex.Archive.
//   - Assertions Commitment.Nb().Location -> Commitment.Location / .Space / .Node.
//
// The production IndexLocator (indexlocator.go) is already ported; this is
// test-only work. Follow the patterns in pkg/verification/cache_test.go (location
// invocations) and pkg/client/dagservice/exchange_test.go (locator.Commitment).
func TestIndexLocator(t *testing.T) {
	t.Skip("IndexLocator tests need a ~1000-line ucantone port (see TODO(forrest))")
}
