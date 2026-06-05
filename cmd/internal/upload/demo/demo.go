package demo

import (
	"context"
	"crypto/sha512"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"time"

	"github.com/fil-forge/libforge/commands"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/multikey"
	"github.com/fil-forge/ucantone/multikey/ed25519"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	"github.com/fil-forge/guppy/cmd/internal/upload/ui"
	"github.com/fil-forge/guppy/internal/fakefs"
	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/preparation"
	"github.com/fil-forge/guppy/pkg/preparation/sqlrepo"
	"github.com/fil-forge/guppy/pkg/preparation/storacha"
)

type changingFS struct {
	fs.FS
	changingFile  string
	count         int
	changeModTime bool
	changeData    bool
}

type seekerFile interface {
	fs.File
	io.Seeker
}

type changingFile struct {
	seekerFile
	fs.FileInfo
	parent changingFS
}

type changingDir struct {
	changingFile
	fs fs.FS
}

func (fsys *changingFS) Open(name string) (fs.File, error) {
	f, err := fsys.FS.Open(name)
	if err != nil {
		return nil, err
	}
	sf, ok := f.(seekerFile)
	if !ok {
		return nil, fmt.Errorf("file %s is not seekable", name)
	}

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	cf := changingFile{
		seekerFile: sf,
		FileInfo:   info,
		parent:     *fsys,
	}

	if name == fsys.changingFile {
		defer func() { fsys.count++ }()
		return cf, nil
	}

	if name == path.Dir(fsys.changingFile) {
		subFS, err := fs.Sub(fsys, name)
		if err != nil {
			return nil, err
		}
		return changingDir{
			changingFile: cf,
			fs:           subFS,
		}, nil
	}

	return f, nil
}

func (f changingFile) Read(b []byte) (int, error) {
	n, err := f.seekerFile.Read(b)
	if err != nil && err != io.EOF {
		return n, err
	}

	if f.parent.changeData && n > 0 {
		// Just change the first byte.
		b[0] = (b[0] + byte(f.parent.count))
	}

	return n, err
}

func (f changingFile) Stat() (fs.FileInfo, error) {
	return f, nil
}

func (d changingDir) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := d.seekerFile.(fs.ReadDirFile).ReadDir(n)
	if err != nil {
		return nil, err
	}

	var changingDEs []fs.DirEntry
	for _, entry := range entries {
		f, err := d.fs.Open(entry.Name())
		if err != nil {
			return nil, err
		}
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		f.Close()
		changingDEs = append(changingDEs, fs.FileInfoToDirEntry(info))

	}
	return changingDEs, nil
}

func (f changingFile) ModTime() time.Time {
	original := f.FileInfo.ModTime()
	if f.parent.changeModTime {
		newTime := original.Add(time.Duration(f.parent.count) * time.Minute)
		return newTime
	}

	return original
}

func newChangingFS(fsys fs.FS, changeModTime, changeData bool) (fs.FS, error) {
	var secondDir string
	root, err := fsys.Open(".")
	if err != nil {
		return nil, err
	}
	defer root.Close()
	rootEntries, err := root.(fs.ReadDirFile).ReadDir(-1)
	if err != nil {
		return nil, err
	}
	for range 2 {
		for _, entry := range rootEntries {
			if entry.IsDir() {
				secondDir = entry.Name()
				break
			}
		}
	}
	if secondDir == "" {
		return nil, fmt.Errorf("no directories found in root")
	}

	var lastDirFirstFile string
	lastDirF, err := fsys.Open(secondDir)
	if err != nil {
		return nil, err
	}
	defer lastDirF.Close()
	lastDirEntries, err := lastDirF.(fs.ReadDirFile).ReadDir(-1)
	if err != nil {
		return nil, err
	}
	for _, entry := range lastDirEntries {
		if !entry.IsDir() {
			lastDirFirstFile = entry.Name()
			break
		}
	}
	if lastDirFirstFile == "" {
		return nil, fmt.Errorf("no files found in last directory %s", secondDir)
	}

	return &changingFS{
		FS:            fsys,
		changingFile:  path.Join(secondDir, lastDirFirstFile),
		changeModTime: changeModTime,
		changeData:    changeData,
	}, nil
}

func Demo(ctx context.Context, repo *sqlrepo.Repo, spaceName string, alterMetadata, alterData bool) error {
	// Derive a deterministic space key from the name so re-runs use the same space.
	seed := sha512.Sum512_256([]byte(spaceName))
	space, err := ed25519.FromRaw(seed[:])
	if err != nil {
		return fmt.Errorf("creating space key: %w", err)
	}
	spaceDID := space.KeyDID()

	service, err := ed25519.GenerateIssuer()
	if err != nil {
		return fmt.Errorf("creating service key: %w", err)
	}

	fsys, err := newChangingFS(fakefs.New(0), alterMetadata, alterData)
	if err != nil {
		return fmt.Errorf("creating changing FS: %w", err)
	}

	api := preparation.NewAPI(repo, &demoClient{service: service}, preparation.WithGetLocalFSForPathFn(func(string) (fs.FS, error) {
		return fsys, nil
	}))

	uploads, err := api.FindOrCreateUploads(ctx, spaceDID)
	if err != nil {
		return fmt.Errorf("creating uploads: %w", err)
	}
	if len(uploads) == 0 {
		if _, err := api.FindOrCreateSpace(ctx, spaceDID, spaceDID.String()); err != nil {
			return fmt.Errorf("creating space: %w", err)
		}
		source, err := api.CreateSource(ctx, ".", ".")
		if err != nil {
			return fmt.Errorf("creating source: %w", err)
		}
		if err := repo.AddSourceToSpace(ctx, spaceDID, source.ID()); err != nil {
			return fmt.Errorf("adding source to space: %w", err)
		}
		uploads, err = api.FindOrCreateUploads(ctx, spaceDID)
		if err != nil {
			return fmt.Errorf("creating uploads: %w", err)
		}
	}

	return ui.RunUploadUI(ctx, repo, api, uploads, false, nil)
}

// demoClient is a fake storacha.Client that accepts blobs locally (no network),
// so the demo can exercise the full scan -> DAG -> shard -> upload pipeline
// without real credentials or a service.
type demoClient struct {
	service multikey.Issuer
}

var _ storacha.Client = (*demoClient)(nil)

func (d *demoClient) BlobAdd(ctx context.Context, content io.Reader, space did.DID, options ...client.BlobAddOption) (client.AddedBlob, error) {
	cfg := client.NewBlobAddConfig(options...)
	data, err := io.ReadAll(content)
	if err != nil {
		return client.AddedBlob{}, fmt.Errorf("reading blob content: %w", err)
	}
	digest := cfg.PrecomputedDigest
	if len(digest) == 0 {
		digest, err = multihash.Sum(data, multihash.SHA2_256, -1)
		if err != nil {
			return client.AddedBlob{}, fmt.Errorf("hashing blob: %w", err)
		}
	}
	blobURL, err := url.Parse("https://demo.example/blob/" + digestutil.Format(digest))
	if err != nil {
		return client.AddedBlob{}, err
	}
	location, err := assertcmds.Location.Invoke(
		d.service,
		d.service.DID(),
		&assertcmds.LocationArguments{
			Space:    space,
			Content:  digest,
			Location: []commands.CborURL{commands.CborURL(*blobURL)},
		},
	)
	if err != nil {
		return client.AddedBlob{}, fmt.Errorf("creating location commitment: %w", err)
	}
	return client.AddedBlob{Digest: digest, Size: uint64(len(data)), Location: location}, nil
}

func (d *demoClient) IndexAdd(ctx context.Context, indexCID cid.Cid, space did.DID) error {
	return nil
}

func (d *demoClient) UploadAdd(ctx context.Context, space did.DID, root cid.Cid, shards []cid.Cid, index *cid.Cid) (*uploadcmds.AddOK, error) {
	return &uploadcmds.AddOK{}, nil
}
