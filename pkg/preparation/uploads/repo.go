package uploads

import (
	"context"

	dagmodel "github.com/fil-forge/guppy/pkg/preparation/dags/model"
	"github.com/fil-forge/guppy/pkg/preparation/types/id"
	uploadmodel "github.com/fil-forge/guppy/pkg/preparation/uploads/model"
	"github.com/fil-forge/ucantone/did"
	"github.com/ipfs/go-cid"
)

type Repo interface {
	// GetUploadByID retrieves an upload by its unique ID.
	GetUploadByID(ctx context.Context, uploadID id.UploadID) (*uploadmodel.Upload, error)
	// FindOrCreateUploads ensures uploads exist for a given space
	FindOrCreateUploads(ctx context.Context, spaceDID did.DID, sourceIDs []id.SourceID) ([]*uploadmodel.Upload, error)
	// UpdateUpload updates the state of an upload in the repository.
	UpdateUpload(ctx context.Context, upload *uploadmodel.Upload) error
	// CIDForFSEntry retrieves the CID for a file system entry by its ID.
	CIDForFSEntry(ctx context.Context, fsEntryID id.FSEntryID) (cid.Cid, error)
	// CreateDAGScan creates a new DAG scan for a file system entry.
	CreateDAGScan(ctx context.Context, fsEntryID id.FSEntryID, isDirectory bool, uploadID id.UploadID, spaceDID did.DID) (dagmodel.DAGScan, error)
	// ListSpaceSources lists all space sources for the given space DID.
	ListSpaceSources(ctx context.Context, spaceDID did.DID) ([]id.SourceID, error)
}
