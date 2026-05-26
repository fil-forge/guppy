package mockclient

import (
	"context"
	"io"
	"net/url"
	"testing"

	"github.com/fil-forge/libforge/commands"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	pdpcmds "github.com/fil-forge/libforge/commands/pdp"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/preparation/storacha"
	blobcmds "github.com/fil-forge/libforge/commands/blob"
)

type MockClient struct {
	T *testing.T

	// If set, these errors will be immediately returned by the corresponding
	// methods, to simulate failure. These may be set and removed between calls.
	BlobAddError  error
	IndexAddError error
	// SpaceBlobReplicateError error
	FilecoinOfferError error
	UploadAddError     error

	// Output
	BlobAddInvocations  []blobAddInvocation
	IndexAddInvocations []indexAddInvocation
	// SpaceBlobReplicateInvocations []spaceBlobReplicateInvocation
	UploadAddInvocations []uploadAddInvocation
}

type blobAddInvocation struct {
	Space     did.DID
	BlobAdded []byte

	ReturnedPDPAccept ucan.Invocation
	ReturnedLocation  ucan.Invocation
}

type indexAddInvocation struct {
	Space    did.DID
	IndexCID cid.Cid
}

type blobReplicateInvocation struct {
	Space              did.DID
	Blob               blobcmds.Blob
	ReplicaCount       uint
	LocationCommitment ucan.Invocation
}

type uploadAddInvocation struct {
	Space  did.DID
	Root   cid.Cid
	Shards []cid.Cid
	Index  *cid.Cid
}

var _ storacha.Client = (*MockClient)(nil)

func (m *MockClient) BlobAdd(ctx context.Context, content io.Reader, space did.DID, options ...client.BlobAddOption) (client.AddedBlob, error) {
	cfg := client.NewBlobAddConfig(options...)

	contentBytes, err := io.ReadAll(content)
	require.NoError(m.T, err, "reading content for BlobAdd")

	if m.BlobAddError != nil {
		m.BlobAddInvocations = append(m.BlobAddInvocations, blobAddInvocation{
			Space:             space,
			BlobAdded:         contentBytes,
			ReturnedPDPAccept: nil,
			ReturnedLocation:  nil,
		})
		return client.AddedBlob{}, m.BlobAddError
	}

	digest := cfg.PrecomputedDigest
	if len(digest) == 0 {
		digest, err = multihash.Sum(contentBytes, multihash.SHA2_256, -1)
		require.NoError(m.T, err, "summing digest for BlobAdd")
	}

	blobURL, err := url.Parse("https://test.example/blob/" + digestutil.Format(digest))
	require.NoError(m.T, err, "parsing blob URL for BlobAdd")

	location, err := assertcmds.Location.Invoke(
		testutil.Service,
		testutil.Service.DID(),
		&assertcmds.LocationArguments{
			Space:    space,
			Content:  digest,
			Location: []commands.CborURL{commands.CborURL(*blobURL)},
		},
	)
	require.NoError(m.T, err, "invoking Location command for BlobAdd")

	dummyPrincipal, err := ed25519.Generate()
	require.NoError(m.T, err)

	pdpAcceptInv, err := pdpcmds.Accept.Invoke(
		dummyPrincipal,
		dummyPrincipal.DID(),
		&pdpcmds.AcceptArguments{
			Blob: digest,
		},
	)
	require.NoError(m.T, err)
	m.BlobAddInvocations = append(m.BlobAddInvocations, blobAddInvocation{
		Space:             space,
		BlobAdded:         contentBytes,
		ReturnedPDPAccept: pdpAcceptInv,
		ReturnedLocation:  location,
	})

	return client.AddedBlob{
		Digest:    digest,
		Size:      uint64(len(contentBytes)),
		Location:  location,
		PDPAccept: pdpAcceptInv,
	}, nil
}

func (m *MockClient) IndexAdd(ctx context.Context, indexCID cid.Cid, space did.DID) error {
	m.IndexAddInvocations = append(m.IndexAddInvocations, indexAddInvocation{
		Space:    space,
		IndexCID: indexCID,
	})

	if m.IndexAddError != nil {
		return m.IndexAddError
	}

	return nil
}

// func (m *MockClient) SpaceBlobReplicate(ctx context.Context, space did.DID, blob types.Blob, replicaCount uint, locationCommitment delegation.Delegation) (spaceblobcap.ReplicateOk, fx.Effects, error) {
// 	m.SpaceBlobReplicateInvocations = append(m.SpaceBlobReplicateInvocations, spaceBlobReplicateInvocation{
// 		Space:              space,
// 		Blob:               blob,
// 		ReplicaCount:       replicaCount,
// 		LocationCommitment: locationCommitment,
// 	})

// 	if m.SpaceBlobReplicateError != nil {
// 		return spaceblobcap.ReplicateOk{}, nil, m.SpaceBlobReplicateError
// 	}

// 	sitePromises := make([]types.Promise, replicaCount)
// 	for i := range sitePromises {
// 		siteDigest, err := multihash.Encode(fmt.Appendf(nil, "test-replicated-site-%d", i), multihash.IDENTITY)
// 		require.NoError(m.T, err, "encoding site digest")
// 		sitePromises[i] = types.Promise{
// 			UcanAwait: types.Await{
// 				Selector: ".out.ok.site",
// 				Link:     cidlink.Link{Cid: cid.NewCidV1(cid.Raw, siteDigest)},
// 			},
// 		}
// 	}
// 	return spaceblobcap.ReplicateOk{Site: sitePromises}, nil, nil
// }

func (m *MockClient) UploadAdd(ctx context.Context, space did.DID, root cid.Cid, shards []cid.Cid, index *cid.Cid) (*uploadcmds.AddOK, error) {
	m.UploadAddInvocations = append(m.UploadAddInvocations, uploadAddInvocation{
		Space:  space,
		Root:   root,
		Shards: shards,
		Index:  index,
	})

	if m.UploadAddError != nil {
		return nil, m.UploadAddError
	}

	return &uploadcmds.AddOK{}, nil
}
