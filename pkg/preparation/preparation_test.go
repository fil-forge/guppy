package preparation_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fil-forge/libforge/blobindex"
	blobcmds "github.com/fil-forge/libforge/commands/blob"
	contentcmds "github.com/fil-forge/libforge/commands/content"
	indexcmds "github.com/fil-forge/libforge/commands/index"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/digestutil"
	libtestutil "github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/server"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/ipld/merkledag"
	unixfile "github.com/ipfs/boxo/ipld/unixfs/file"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	carblockstore "github.com/ipld/go-car/v2/blockstore"
	"github.com/multiformats/go-multicodec"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
	"github.com/fil-forge/guppy/pkg/preparation"
	"github.com/fil-forge/guppy/pkg/preparation/internal/testdb"
	spacesmodel "github.com/fil-forge/guppy/pkg/preparation/spaces/model"
	"github.com/fil-forge/guppy/pkg/preparation/sqlrepo"
	gtypes "github.com/fil-forge/guppy/pkg/preparation/types"
	uploadsmodel "github.com/fil-forge/guppy/pkg/preparation/uploads/model"
)

// NOTE: /blob/replicate and filecoin/offer are commented out in the
// production storacha API, so this end-to-end test only exercises (and asserts)
// the surviving flow: /blob/add (via the ctestutil blob/add harness),
// /index/add, and upload/add. TODO(forrest): restore the replicate / offer
// assertions when those steps are re-enabled.

func randomBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	return b
}

// recordedInvocations captures the index/add and upload/add invocations the
// fake service receives.
type recordedInvocations struct {
	mu         sync.Mutex
	indexAdds  []recordedInvocation[*indexcmds.AddArguments]
	uploadAdds []recordedInvocation[*uploadcmds.AddArguments]
}

type recordedInvocation[Args any] struct {
	subject did.DID
	args    Args
}

func (r *recordedInvocations) recordIndex(subject did.DID, args *indexcmds.AddArguments) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexAdds = append(r.indexAdds, recordedInvocation[*indexcmds.AddArguments]{subject, args})
}

func (r *recordedInvocations) recordUpload(subject did.DID, args *uploadcmds.AddArguments) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.uploadAdds = append(r.uploadAdds, recordedInvocation[*uploadcmds.AddArguments]{subject, args})
}

// prepareTestClient creates a test storacha client backed by an in-process
// service that handles blob/add (via the ctestutil harness), index/add, and
// upload/add. PUTs go through putClient; index/add and upload/add invocations
// are recorded into rec.
func prepareTestClient(t *testing.T, space ed25519.Signer, putClient *http.Client, rec *recordedInvocations) *ctestutil.ClientWithCustomPut {
	t.Helper()
	c := &ctestutil.ClientWithCustomPut{
		Client: libtestutil.Must(ctestutil.Client(t,
			ctestutil.WithBlobAdd(t),
			ctestutil.WithServerRoutes(
				func(deps ctestutil.RouteDeps) server.Route {
					return indexcmds.Add.Route(func(req *binding.Request[*indexcmds.AddArguments], res *binding.Response[*indexcmds.AddOK]) error {
						rec.recordIndex(req.Invocation().Subject(), req.Task().Arguments())
						return res.SetSuccess(&indexcmds.AddOK{})
					})
				},
				func(deps ctestutil.RouteDeps) server.Route {
					return uploadcmds.Add.Route(func(req *binding.Request[*uploadcmds.AddArguments], res *binding.Response[*uploadcmds.AddOK]) error {
						rec.recordUpload(req.Invocation().Subject(), req.Task().Arguments())
						return res.SetSuccess(&uploadcmds.AddOK{})
					})
				},
			),
		))(t),
		PutClient: putClient,
	}

	// The agent needs proofs for each command the pipeline invokes on the space.
	require.NoError(t, c.AddProofs(t.Context(),
		libtestutil.Must(blobcmds.Add.Delegate(space, c.Issuer().DID(), space.DID()))(t),
		libtestutil.Must(contentcmds.Retrieve.Delegate(space, c.Issuer().DID(), space.DID()))(t),
		libtestutil.Must(indexcmds.Add.Delegate(space, c.Issuer().DID(), space.DID()))(t),
		libtestutil.Must(uploadcmds.Add.Delegate(space, c.Issuer().DID(), space.DID()))(t),
	))

	return c
}

func createUpload(t testing.TB, ctx context.Context, sourcePath string, repo *sqlrepo.Repo, spaceDID did.DID, api preparation.API, shardSize uint64) *uploadsmodel.Upload {
	t.Helper()
	_, err := api.FindOrCreateSpace(ctx, spaceDID, "Large Upload Space", spacesmodel.WithShardSize(shardSize))
	require.NoError(t, err)
	source, err := api.CreateSource(ctx, "Large Upload Source", sourcePath)
	require.NoError(t, err)
	err = repo.AddSourceToSpace(ctx, spaceDID, source.ID())
	require.NoError(t, err)
	uploads, err := api.FindOrCreateUploads(ctx, spaceDID)
	require.NoError(t, err)
	require.Len(t, uploads, 1, "expected exactly one upload to be created")
	return uploads[0]
}

func TestExecuteUpload(t *testing.T) {
	t.Run("uploads", func(t *testing.T) {
		space := libtestutil.RandomSigner(t)

		db := testdb.CreateTestDB(t)
		_, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = ON;")
		require.NoError(t, err, "failed to enable foreign keys")
		repo := libtestutil.Must(sqlrepo.New(db))(t)

		aBytes := randomBytes((1 << 16) - 128)
		fsData := map[string][]byte{
			"a":           aBytes,
			"dir1/b":      randomBytes((1 << 16) - 128),
			"dir1/c":      randomBytes((1 << 16) - 128),
			"dir1/dir2/d": randomBytes((1 << 16) - 128),
			// Make one file identical to another to test deduplication.
			"dir1/dir2/a-again": aBytes,
		}

		testFs := prepareFs(t, fsData)

		putClient := ctestutil.NewPutClient()
		rec := &recordedInvocations{}
		c := prepareTestClient(t, space, putClient, rec)

		uploadSourcePath := t.TempDir()
		api := preparation.NewAPI(
			repo,
			c,
			preparation.WithGetLocalFSForPathFn(func(path string) (fs.FS, error) {
				assert.Equal(t, uploadSourcePath, path, "test expects root to be '.'")
				return testFs, nil
			}),
			preparation.WithBlobUploadParallelism(1),
		)

		upload := createUpload(t, t.Context(), uploadSourcePath, repo, space.DID(), api, 1<<16)

		returnedRootCID, err := api.ExecuteUpload(t.Context(), upload)
		require.NoError(t, err)
		require.NotEmpty(t, returnedRootCID, "expected non-empty root CID")

		putBlobs := ctestutil.ReceivedBlobs(putClient)
		require.Equal(t, 6, putBlobs.Size(), "expected 5 shards + 1 index to be added")

		require.Len(t, rec.indexAdds, 1, "expected exactly one index/add invocation")
		require.Equal(t, space.DID(), rec.indexAdds[0].subject)
		require.True(t, rec.indexAdds[0].args.Index.Defined(), "expected index/add to carry an index CID")
		indexCID := rec.indexAdds[0].args.Index

		require.Len(t, rec.uploadAdds, 1, "expected exactly one upload/add invocation")
		require.Equal(t, space.DID(), rec.uploadAdds[0].subject)
		rootCID := rec.uploadAdds[0].args.Root
		require.Equal(t, rootCID, returnedRootCID, "expected returned root CID to match the upload/add")

		foundData := filesData(t.Context(), t, rootCID, indexCID, putBlobs)
		require.True(t, assert.ObjectsAreEqual(fsData, foundData), "expected all files to be present and match")
	})

	t.Run("after an error, can be retried safely", func(t *testing.T) {
		logging.SetLogLevel("preparation/uploads", "dpanic")

		space := libtestutil.RandomSigner(t)

		db := testdb.CreateTestDB(t)
		_, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = ON;")
		require.NoError(t, err, "failed to enable foreign keys")
		repo := libtestutil.Must(sqlrepo.New(db))(t)

		fsData := map[string][]byte{
			"dir1/b":      randomBytes((1 << 16) - 128),
			"a":           randomBytes((1 << 16) - 128),
			"dir1/c":      randomBytes((1 << 16) - 128),
			"dir1/dir2/d": randomBytes((1 << 16) - 128),
		}

		testFs := prepareFs(t, fsData)

		putCount := 0
		putClient := ctestutil.NewPutClient()
		putClient.Transport = &errorableTransport{
			wrapped: putClient.Transport,
			errFn: func(req *http.Request) error {
				data, err := bodyData(req)
				if err != nil {
					return err
				}
				// If it parses as an index, skip it.
				if _, err := blobindex.Extract(bytes.NewReader(data)); err == nil {
					return nil
				}
				putCount++
				if putCount == 3 {
					return assert.AnError
				}
				return nil
			},
		}

		rec := &recordedInvocations{}
		c := prepareTestClient(t, space, putClient, rec)

		uploadSourcePath := t.TempDir()
		api := preparation.NewAPI(
			repo,
			c,
			preparation.WithGetLocalFSForPathFn(func(path string) (fs.FS, error) {
				assert.Equal(t, uploadSourcePath, path, "test expects root to be '.'")
				return testFs, nil
			}),
			preparation.WithBlobUploadParallelism(1),
		)

		upload := createUpload(t, t.Context(), uploadSourcePath, repo, space.DID(), api, 1<<16)

		// First attempt should fail (on the third PUT).
		_, err = api.ExecuteUpload(t.Context(), upload)
		var blobUploadErrors gtypes.BlobUploadErrors
		require.ErrorAs(t, err, &blobUploadErrors, "expected a BlobUploadErrors error")
		underlying := blobUploadErrors.Unwrap()
		require.Len(t, underlying, 1, "expected exactly one underlying error")
		require.ErrorIs(t, underlying[0], assert.AnError, "expected error on third PUT request")

		putBlobs := ctestutil.ReceivedBlobs(putClient)
		require.GreaterOrEqual(t, putBlobs.Size(), 2, "expected at least 2 shards so far")
		require.Less(t, putBlobs.Size(), 6, "expected fewer than 5 shards + 1 index so far")
		require.Len(t, rec.uploadAdds, 0, "expected upload/add not to have been called yet")

		t.Log("Retrying the upload after error...")
		upload, err = api.GetUploadByID(t.Context(), upload.ID())
		require.NoError(t, err)

		returnedRootCID, err := api.ExecuteUpload(t.Context(), upload)
		require.NoError(t, err, "expected upload to succeed on retry")
		require.NotEmpty(t, returnedRootCID, "expected non-empty root CID")

		putBlobs = ctestutil.ReceivedBlobs(putClient)
		require.Equal(t, 6, putBlobs.Size(), "expected 5 shards + 1 index in the end")

		require.Len(t, rec.indexAdds, 1, "expected one index/add invocation")
		require.Equal(t, space.DID(), rec.indexAdds[0].subject)
		require.True(t, rec.indexAdds[0].args.Index.Defined())
		indexCID := rec.indexAdds[0].args.Index

		require.Len(t, rec.uploadAdds, 1, "expected one upload/add invocation")
		require.Equal(t, space.DID(), rec.uploadAdds[0].subject)
		rootCID := rec.uploadAdds[0].args.Root
		require.Equal(t, rootCID, returnedRootCID)

		foundData := filesData(t.Context(), t, rootCID, indexCID, putBlobs)
		require.True(t, assert.ObjectsAreEqual(fsData, foundData), "expected all files to be present and match")
	})
}

func prepareFs(t *testing.T, files map[string][]byte) afero.IOFS {
	t.Helper()
	memFS := afero.NewMemMapFs()
	for path, data := range files {
		require.NoError(t, memFS.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, afero.WriteFile(memFS, path, data, 0644), "failed to write file %s", path)
	}
	memIOFS := afero.NewIOFS(memFS)
	fs.WalkDir(memIOFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return memFS.Chtimes(path, time.Now(), time.Now())
	})
	return memIOFS
}

func filesData(ctx context.Context, t *testing.T, rootCID cid.Cid, indexCID cid.Cid, shards ctestutil.BlobMap) map[string][]byte {
	bs, err := newIndexAndShardsBlockstore(indexCID, shards)
	require.NoError(t, err)

	blockserv := blockservice.New(bs, nil)
	dagserv := merkledag.NewDAGService(blockserv)
	rootNode, err := dagserv.Get(ctx, rootCID)
	require.NoError(t, err)
	rootFileNode, err := unixfile.NewUnixfsFile(ctx, dagserv, rootNode)
	require.NoError(t, err)

	foundData := make(map[string][]byte)
	err = files.Walk(rootFileNode, func(fpath string, fnode files.Node) error {
		file, ok := fnode.(files.File)
		if !ok {
			return nil // skip directories
		}
		data, err := io.ReadAll(file)
		require.NoError(t, err)
		foundData[fpath] = data
		return nil
	})
	require.NoError(t, err)
	return foundData
}

// indexAndShardsBlockstore serves blocks by slicing shard CARs according to a
// ShardedDagIndex. Only Get is implemented; it is for testing only.
type indexAndShardsBlockstore struct {
	index  blobindex.ShardedDagIndex
	shards ctestutil.BlobMap
}

var _ carblockstore.Blockstore = (*indexAndShardsBlockstore)(nil)

func newIndexAndShardsBlockstore(indexCID cid.Cid, shards ctestutil.BlobMap) (*indexAndShardsBlockstore, error) {
	if indexCID.Prefix().Codec != uint64(multicodec.Car) {
		return nil, fmt.Errorf("expected index CID codec 0x%x (CAR), got 0x%x", multicodec.Car, indexCID.Prefix().Codec)
	}
	indexDigest := indexCID.Hash()
	if !shards.Has(indexDigest) {
		return nil, fmt.Errorf("index CID %s (digest %s) not found in provided shards", indexCID, digestutil.Format(indexDigest))
	}
	index, err := blobindex.Extract(bytes.NewReader(shards.Get(indexDigest)))
	if err != nil {
		return nil, fmt.Errorf("extracting index from CAR: %v", err)
	}
	return &indexAndShardsBlockstore{index: index, shards: shards}, nil
}

func (c *indexAndShardsBlockstore) Get(ctx context.Context, key cid.Cid) (blocks.Block, error) {
	for shardDigest, sliceMap := range c.index.Shards().Iterator() {
		for sliceDigest, position := range sliceMap.Iterator() {
			if bytes.Equal(key.Hash(), sliceDigest) {
				if !c.shards.Has(shardDigest) {
					return nil, fmt.Errorf("shard with digest %s not found", digestutil.Format(shardDigest))
				}
				shardBlob := c.shards.Get(shardDigest)
				return blocks.NewBlockWithCid(shardBlob[position.Start:position.End+1], key)
			}
		}
	}
	return nil, format.ErrNotFound{Cid: key}
}

func (c *indexAndShardsBlockstore) DeleteBlock(context.Context, cid.Cid) error {
	panic("not implemented")
}
func (c *indexAndShardsBlockstore) Has(context.Context, cid.Cid) (bool, error) {
	panic("not implemented")
}
func (c *indexAndShardsBlockstore) GetSize(context.Context, cid.Cid) (int, error) {
	panic("not implemented")
}
func (c *indexAndShardsBlockstore) Put(context.Context, blocks.Block) error { panic("not implemented") }
func (c *indexAndShardsBlockstore) PutMany(context.Context, []blocks.Block) error {
	panic("not implemented")
}
func (c *indexAndShardsBlockstore) AllKeysChan(context.Context) (<-chan cid.Cid, error) {
	panic("not implemented")
}
func (c *indexAndShardsBlockstore) HashOnRead(bool) { panic("not implemented") }

// errorableTransport wraps an http.RoundTripper to optionally inject an error.
type errorableTransport struct {
	wrapped http.RoundTripper
	errFn   func(req *http.Request) error
}

var _ http.RoundTripper = (*errorableTransport)(nil)
var _ ctestutil.BlobReceiver = (*errorableTransport)(nil)

func (e *errorableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if e.errFn != nil {
		if err := e.errFn(req); err != nil {
			return nil, err
		}
	}
	return e.wrapped.RoundTrip(req)
}

func (e *errorableTransport) ReceivedBlobs() ctestutil.BlobMap {
	if receiver, ok := e.wrapped.(ctestutil.BlobReceiver); ok {
		return receiver.ReceivedBlobs()
	}
	return nil
}

// bodyData reads and restores an HTTP request body.
func bodyData(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}
