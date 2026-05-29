package storacha_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	stestutil "github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/did"
	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	gtestutil "github.com/fil-forge/guppy/internal/testutil"
	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/preparation/blobs"
	"github.com/fil-forge/guppy/pkg/preparation/blobs/model"
	"github.com/fil-forge/guppy/pkg/preparation/internal/mockclient"
	"github.com/fil-forge/guppy/pkg/preparation/internal/testdb"
	"github.com/fil-forge/guppy/pkg/preparation/internal/testutil"
	spacesmodel "github.com/fil-forge/guppy/pkg/preparation/spaces/model"
	"github.com/fil-forge/guppy/pkg/preparation/sqlrepo"
	"github.com/fil-forge/guppy/pkg/preparation/storacha"
	"github.com/fil-forge/guppy/pkg/preparation/types/id"
)

// NOTE: the storacha upload flow is mid-refactor in the ucantone migration:
// `space/blob/replicate` and `filecoin/offer` are commented out in the
// production API, and PostProcessUploadedShards is currently a no-op. These
// tests therefore assert only the surviving behavior — `space/blob/add`,
// `space/index/add`, and `upload/add`. TODO(forrest): restore the replicate /
// offer / rich space/index/add assertions once those steps are re-enabled.

// commP is not defined for inputs shorter than 127 bytes, so add 127 bytes of
// padding to every "CAR" to make sure it's definitely long enough.
var padding = bytes.Repeat([]byte{0}, 127)

func TestAddShardsForUpload(t *testing.T) {
	t.Run("`space/blob/add`s a CAR for each shard", func(t *testing.T) {
		db := testdb.CreateTestDB(t)
		repo := stestutil.Must(sqlrepo.New(db))(t)
		spaceDID, err := did.Parse("did:storacha:space:example")
		require.NoError(t, err)
		mclient := mockclient.MockClient{T: t}

		shardReadersClosed := map[id.ShardID]struct{}{}
		carForShard := func(ctx context.Context, shardID id.ShardID) (io.ReadCloser, error) {
			return &blobReadCloser{
				blobReadersClosed: shardReadersClosed,
				blobID:            shardID,
				Reader:            bytes.NewReader(fmt.Append(nil, "CAR OF SHARD: ", shardID, padding)),
			}, nil
		}

		api := storacha.API{
			Repo:                  repo,
			Client:                &mclient,
			ReaderForShard:        carForShard,
			BlobUploadParallelism: 1,
			Replicas:              3,
		}

		blobsApi := blobs.API{
			Repo:         repo,
			ShardEncoder: blobs.NewCAREncoder(),
		}

		upload, source := testutil.CreateUpload(t, repo, spaceDID, spacesmodel.WithShardSize(1<<16))

		// Add enough nodes to close one shard and create a second one.
		testutil.AddNodeToUploadShards(t, repo, blobsApi, upload.ID(), source.ID(), spaceDID, nil, 1<<14)
		testutil.AddNodeToUploadShards(t, repo, blobsApi, upload.ID(), source.ID(), spaceDID, nil, 1<<14)
		testutil.AddNodeToUploadShards(t, repo, blobsApi, upload.ID(), source.ID(), spaceDID, nil, 1<<15)

		shards, err := repo.ShardsForUploadByState(t.Context(), upload.ID(), model.BlobStateClosed)
		require.NoError(t, err)
		require.Len(t, shards, 1)
		firstShard := shards[0]

		shards, err = repo.ShardsForUploadByState(t.Context(), upload.ID(), model.BlobStateOpen)
		require.NoError(t, err)
		require.Len(t, shards, 1)
		secondShard := shards[0]

		// Upload shards that are ready to go.
		err = api.AddShardsForUpload(t.Context(), upload.ID(), spaceDID, nil)
		require.NoError(t, err)

		// Reload shards
		firstShard, err = repo.GetShardByID(t.Context(), firstShard.ID())
		require.NoError(t, err)
		secondShard, err = repo.GetShardByID(t.Context(), secondShard.ID())
		require.NoError(t, err)
		require.Equal(t, model.BlobStateUploaded, firstShard.State(), "expected first shard to be marked as uploaded now")
		require.Equal(t, model.BlobStateOpen, secondShard.State(), "expected second shard to remain open")

		// This run should `space/blob/add` the first, closed shard.
		expectedData := fmt.Append(nil, "CAR OF SHARD: ", firstShard.ID(), padding)
		require.Len(t, mclient.BlobAddInvocations, 1)
		require.Equal(t, expectedData, mclient.BlobAddInvocations[0].BlobAdded)
		require.Equal(t, spaceDID, mclient.BlobAddInvocations[0].Space)

		// It should have closed the first shard's reader during the add.
		require.Contains(t, shardReadersClosed, firstShard.ID(), "expected first shard reader to be closed now")

		// Post-processing is currently a no-op for shards (replicate/offer are
		// disabled), so the shard remains in the uploaded state.
		err = api.PostProcessUploadedShards(t.Context(), upload.ID(), spaceDID)
		require.NoError(t, err)
		firstShard, err = repo.GetShardByID(t.Context(), firstShard.ID())
		require.NoError(t, err)
		require.Equal(t, model.BlobStateUploaded, firstShard.State())

		// Now close the upload shards and run it again.
		err = blobsApi.CloseUploadShards(t.Context(), upload.ID(), nil)
		require.NoError(t, err)
		err = api.AddShardsForUpload(t.Context(), upload.ID(), spaceDID, nil)
		require.NoError(t, err)

		// Reload second shard
		secondShard, err = repo.GetShardByID(t.Context(), secondShard.ID())
		require.NoError(t, err)
		require.Equal(t, model.BlobStateUploaded, secondShard.State(), "expected second shard to be marked as uploaded now")

		// This run should `space/blob/add` the second, newly closed shard.
		require.Len(t, mclient.BlobAddInvocations, 2)
		require.Equal(t, fmt.Append(nil, "CAR OF SHARD: ", secondShard.ID(), padding), mclient.BlobAddInvocations[1].BlobAdded)
		require.Equal(t, spaceDID, mclient.BlobAddInvocations[1].Space)

		// It should have closed the second shard's reader.
		require.Contains(t, shardReadersClosed, secondShard.ID(), "expected second shard reader to be closed now")
	})

	t.Run("does not `space/blob/add` again on retry once it succeeds", func(t *testing.T) {
		db := testdb.CreateTestDB(t)
		repo := stestutil.Must(sqlrepo.New(db))(t)
		spaceDID, err := did.Parse("did:storacha:space:example")
		require.NoError(t, err)
		mclient := mockclient.MockClient{T: t}

		shardReadersClosed := map[id.ShardID]struct{}{}
		carForShard := func(ctx context.Context, shardID id.ShardID) (io.ReadCloser, error) {
			return &blobReadCloser{
				blobReadersClosed: shardReadersClosed,
				blobID:            shardID,
				Reader:            bytes.NewReader(fmt.Append(nil, "CAR OF SHARD: ", shardID, padding)),
			}, nil
		}

		api := storacha.API{
			Repo:                  repo,
			Client:                &mclient,
			ReaderForShard:        carForShard,
			BlobUploadParallelism: 1,
			Replicas:              3,
		}

		blobsApi := blobs.API{
			Repo:         repo,
			ShardEncoder: blobs.NewCAREncoder(),
		}

		upload, source := testutil.CreateUpload(t, repo, spaceDID, spacesmodel.WithShardSize(1<<16))
		testutil.AddNodeToUploadShards(t, repo, blobsApi, upload.ID(), source.ID(), spaceDID, nil, 1<<14)
		err = blobsApi.CloseUploadShards(t.Context(), upload.ID(), nil)
		require.NoError(t, err)

		// First attempt: `space/blob/add` fails.
		mclient.BlobAddError = fmt.Errorf("simulated BlobAdd error")
		err = api.AddShardsForUpload(t.Context(), upload.ID(), spaceDID, nil)
		require.ErrorContains(t, err, "simulated BlobAdd error")
		require.Len(t, mclient.BlobAddInvocations, 1)
		require.Len(t, shardReadersClosed, 1, "expected shard reader to be closed, even though it failed")
		for shardID := range shardReadersClosed {
			delete(shardReadersClosed, shardID)
		}

		// Retry: `space/blob/add` succeeds, so it adds again.
		mclient.BlobAddError = nil
		err = api.AddShardsForUpload(t.Context(), upload.ID(), spaceDID, nil)
		require.NoError(t, err)
		require.Len(t, mclient.BlobAddInvocations, 2)

		// Run again: the shard now has a location, so it is not re-added.
		err = api.AddShardsForUpload(t.Context(), upload.ID(), spaceDID, nil)
		require.NoError(t, err)
		require.Len(t, mclient.BlobAddInvocations, 2)
	})
}

func TestAddIndexesForUpload(t *testing.T) {
	t.Run("`space/blob/add`s and `space/index/add`s index CARs", func(t *testing.T) {
		logging.SetLogLevel("preparation/storacha", "warn")
		db := testdb.CreateTestDB(t)
		repo := stestutil.Must(sqlrepo.New(db))(t)
		spaceDID, err := did.Parse("did:storacha:space:example")
		require.NoError(t, err)
		mclient := mockclient.MockClient{T: t}

		indexReadersClosed := map[id.ShardID]struct{}{}
		carForIndex := func(ctx context.Context, indexID id.IndexID) (io.ReadCloser, error) {
			return &blobReadCloser{
				blobReadersClosed: indexReadersClosed,
				blobID:            indexID,
				Reader:            bytes.NewReader(fmt.Append(nil, "CAR OF INDEX: ", indexID, padding))}, nil
		}

		api := storacha.API{
			Repo:                  repo,
			Client:                &mclient,
			ReaderForIndex:        carForIndex,
			BlobUploadParallelism: 1,
			Replicas:              3,
		}

		blobsApi := blobs.API{
			Repo:             repo,
			ShardEncoder:     blobs.NewCAREncoder(),
			MaxNodesPerIndex: 3,
		}

		upload, source := testutil.CreateUpload(t, repo, spaceDID, spacesmodel.WithShardSize(1<<7))
		rootCID := stestutil.RandomCID(t)
		upload.SetRootCID(rootCID)
		err = repo.UpdateUpload(t.Context(), upload)
		require.NoError(t, err)

		var indexes []*model.Index
		recordClosedIndex := func(index *model.Index) error {
			indexes = append(indexes, index)
			return nil
		}

		var shards []*model.Shard
		recordClosedShard := func(shard *model.Shard) error {
			blobsApi.AddShardToUploadIndexes(t.Context(), upload.ID(), shard.ID(), recordClosedIndex)
			shards = append(shards, shard)
			return nil
		}

		// Add enough nodes to create three shards and two indexes.
		for range 5 {
			testutil.AddNodeToUploadShards(t, repo, blobsApi, upload.ID(), source.ID(), spaceDID, recordClosedShard, 1<<3)
		}
		err = blobsApi.CloseUploadShards(t.Context(), upload.ID(), recordClosedShard)
		require.NoError(t, err)
		require.Len(t, shards, 3)
		require.Len(t, indexes, 1)

		err = api.AddIndexesForUpload(t.Context(), upload.ID(), spaceDID, nil)
		require.NoError(t, err)

		firstIndex, err := repo.GetIndexByID(t.Context(), indexes[0].ID())
		require.NoError(t, err)
		require.Equal(t, model.BlobStateUploaded, firstIndex.State(), "expected first index to be marked as uploaded now")

		// This run should `space/blob/add` the first, closed index.
		expectedData := fmt.Append(nil, "CAR OF INDEX: ", firstIndex.ID(), padding)
		require.Len(t, mclient.BlobAddInvocations, 1)
		require.Equal(t, expectedData, mclient.BlobAddInvocations[0].BlobAdded)
		require.Equal(t, spaceDID, mclient.BlobAddInvocations[0].Space)

		// Post-processing `space/index/add`s the index and marks it added.
		err = api.PostProcessUploadedIndexes(t.Context(), upload.ID(), spaceDID)
		require.NoError(t, err)

		firstIndex, err = repo.GetIndexByID(t.Context(), indexes[0].ID())
		require.NoError(t, err)
		require.Equal(t, model.BlobStateAdded, firstIndex.State(), "expected first index to be marked as added now")

		require.Len(t, mclient.IndexAddInvocations, 1)
		require.Equal(t, spaceDID, mclient.IndexAddInvocations[0].Space)
		require.Equal(t, firstIndex.CID(), mclient.IndexAddInvocations[0].IndexCID)

		// It should have closed the first index's reader.
		require.Contains(t, indexReadersClosed, firstIndex.ID(), "expected first index reader to be closed now")

		// Now close the upload indexes and run it again.
		err = blobsApi.CloseUploadIndexes(t.Context(), upload.ID(), recordClosedIndex)
		require.NoError(t, err)
		require.Len(t, indexes, 2)
		err = api.AddIndexesForUpload(t.Context(), upload.ID(), spaceDID, nil)
		require.NoError(t, err)

		secondIndex, err := repo.GetIndexByID(t.Context(), indexes[1].ID())
		require.NoError(t, err)
		require.Equal(t, model.BlobStateUploaded, secondIndex.State(), "expected second index to be uploaded now")

		require.Len(t, mclient.BlobAddInvocations, 2)
		require.Equal(t, fmt.Append(nil, "CAR OF INDEX: ", secondIndex.ID(), padding), mclient.BlobAddInvocations[1].BlobAdded)

		err = api.PostProcessUploadedIndexes(t.Context(), upload.ID(), spaceDID)
		require.NoError(t, err)

		secondIndex, err = repo.GetIndexByID(t.Context(), indexes[1].ID())
		require.NoError(t, err)
		require.Equal(t, model.BlobStateAdded, secondIndex.State(), "expected second index to be marked as added now")

		require.Len(t, mclient.IndexAddInvocations, 2)
		require.Equal(t, secondIndex.CID(), mclient.IndexAddInvocations[1].IndexCID)

		require.Contains(t, indexReadersClosed, secondIndex.ID(), "expected second index reader to be closed now")
	})
}

func TestAddStorachaUploadForUpload(t *testing.T) {
	t.Run("`upload/add`s the root and shards", func(t *testing.T) {
		db := testdb.CreateTestDB(t)
		repo := stestutil.Must(sqlrepo.New(db))(t)
		spaceDID, err := did.Parse("did:storacha:space:example")
		require.NoError(t, err)
		mclient := mockclient.MockClient{}

		api := storacha.API{
			Repo:                  repo,
			Client:                &mclient,
			BlobUploadParallelism: 1,
			Replicas:              3,
		}

		upload, _ := testutil.CreateUpload(t, repo, spaceDID, spacesmodel.WithShardSize(1<<16))

		rootLink := stestutil.RandomCID(t)

		data1 := append([]byte("CAR OF SHARD 1"), padding...)
		shard1Digest, err := multihash.Sum(data1, multihash.SHA2_256, -1)
		require.NoError(t, err)
		commp := &commp.Calc{}
		commp.Write(data1)
		piece1CID, err := commcid.DataCommitmentToPieceCidv2(commp.Sum(nil), uint64(len(data1)))
		require.NoError(t, err)
		shard1, err := repo.CreateShard(t.Context(), upload.ID(), 10, nil, nil)
		require.NoError(t, err)
		err = shard1.Close(shard1Digest, piece1CID)
		require.NoError(t, err)
		err = shard1.BlobAdded(client.AddedBlob{
			Location:  gtestutil.RandomLocationInvocation(t),
			PDPAccept: nil,
			Digest:    shard1.Digest(),
			Size:      shard1.Size(),
		})
		require.NoError(t, err)
		err = shard1.Added()
		require.NoError(t, err)
		err = repo.UpdateShard(t.Context(), shard1)
		require.NoError(t, err)

		data2 := append([]byte("CAR OF SHARD 2"), padding...)
		shard2Digest, err := multihash.Sum(data2, multihash.SHA2_256, -1)
		require.NoError(t, err)
		commp.Write(data2)
		piece2CID, err := commcid.DataCommitmentToPieceCidv2(commp.Sum(nil), uint64(len(data2)))
		require.NoError(t, err)
		shard2, err := repo.CreateShard(t.Context(), upload.ID(), 20, nil, nil)
		require.NoError(t, err)
		err = shard2.Close(shard2Digest, piece2CID)
		require.NoError(t, err)
		err = shard2.BlobAdded(client.AddedBlob{
			Location:  gtestutil.RandomLocationInvocation(t),
			PDPAccept: nil,
			Digest:    shard2.Digest(),
			Size:      shard2.Size(),
		})
		require.NoError(t, err)
		err = shard2.Added()
		require.NoError(t, err)
		err = repo.UpdateShard(t.Context(), shard2)
		require.NoError(t, err)

		upload.SetRootCID(rootLink)
		err = repo.UpdateUpload(t.Context(), upload)
		require.NoError(t, err)

		err = api.AddStorachaUploadForUpload(t.Context(), upload.ID(), spaceDID)
		require.NoError(t, err)

		require.Len(t, mclient.UploadAddInvocations, 1)
		require.Equal(t, spaceDID, mclient.UploadAddInvocations[0].Space)
		require.Equal(t, rootLink, mclient.UploadAddInvocations[0].Root)
		require.ElementsMatch(t, []cid.Cid{
			cid.NewCidV1(uint64(multicodec.Car), shard1Digest),
			cid.NewCidV1(uint64(multicodec.Car), shard2Digest),
		}, mclient.UploadAddInvocations[0].Shards)
	})
}

type blobReadCloser struct {
	io.Reader
	blobReadersClosed map[id.ID]struct{}
	blobID            id.ID
}

func (sc *blobReadCloser) Close() error {
	sc.blobReadersClosed[sc.blobID] = struct{}{}
	return nil
}
