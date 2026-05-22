package demo

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"time"

	"github.com/fil-forge/guppy/pkg/preparation/sqlrepo"
)

type nullTransport struct{}

func (t nullTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	time.Sleep(1 * time.Second)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
	}, nil
}

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
	// TODO(forrest): this demo stands up a go-ucanto test server via the client
	// test harness (ctestutil) using the removed client.WithPrincipal and
	// go-ucanto server.Provide handlers. The client and test harness were upgraded
	// to ucantone; porting needs the new test-server API — confirm intent with
	// Alan. Disabled until then. (The changing-FS helpers above are left intact.)
	return fmt.Errorf("upload demo is temporarily disabled during the client upgrade to ucantone (TODO(forrest))")
}
