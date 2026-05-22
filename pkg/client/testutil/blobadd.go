package testutil

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/fil-forge/go-libstoracha/bytemap"
	"github.com/fil-forge/go-ucanto/testing/helpers"
	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/libforge/commands"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	blobcmds "github.com/fil-forge/libforge/commands/blob"
	httpcmds "github.com/fil-forge/libforge/commands/http"
	pdpcmds "github.com/fil-forge/libforge/commands/pdp"
	ucancmds "github.com/fil-forge/libforge/commands/ucan"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/libforge/piece"
	receiptclient "github.com/fil-forge/libforge/receipt"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ipld/datamodel"
	"github.com/fil-forge/ucantone/principal"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/server"
	"github.com/fil-forge/ucantone/transport"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/container"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/fil-forge/ucantone/ucan/promise"
	"github.com/fil-forge/ucantone/ucan/receipt"
	"github.com/filecoin-project/go-data-segment/merkletree"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

const storageURLPrefix = "https://storage.example/store/"

func executeAllocate(
	t *testing.T,
	allocateArgs *blobcmds.AllocateArguments,
	allocateInv ucan.Invocation,
	storageProvider ucan.Signer,
	blobSize uint64,
) ucan.Receipt {
	putBlobURL := testutil.Must(url.Parse(storageURLPrefix + digestutil.Format(allocateArgs.Blob.Digest)))(t)
	return testutil.Must(
		receipt.IssueOK(storageProvider, allocateInv.Task().Link(), &blobcmds.AllocateOK{
			Size: blobSize,
			Address: &blobcmds.BlobAddress{
				URL:     commands.CborURL(*putBlobURL),
				Headers: map[string]string{"some-header": "some-value"},
				Expires: time.Now().Add(1 * time.Minute).Unix(),
			},
		}),
	)(t)
}

func invokePut(
	t *testing.T,
	blobProvider principal.Signer,
	blobDigest multihash.Multihash,
	blobSize uint64,
	allocateTask cid.Cid,
) ucan.Invocation {
	return testutil.Must(httpcmds.Put.Invoke(
		blobProvider,
		blobProvider.DID(),
		&httpcmds.PutArguments{
			Body: blobcmds.Blob{
				Digest: blobDigest,
				Size:   blobSize,
			},
			Destination: promise.AwaitOK{Task: allocateTask},
		},
		invocation.WithAudience(blobProvider.DID()),
		invocation.WithMetadata(
			datamodel.Map{
				"keys": datamodel.Map{
					"id": blobProvider.DID().String(),
					"keys": datamodel.Map{
						blobProvider.DID().String(): blobProvider.Bytes(),
					},
				},
			},
		),
	))(t)
}

func invokeAccept(
	t *testing.T,
	serviceID ucan.Signer,
	storageProvider ucan.Signer,
	space did.DID,
	blobDigest multihash.Multihash,
	blobSize uint64,
	httpPutTask cid.Cid,
) ucan.Invocation {
	return testutil.Must(
		blobcmds.Accept.Invoke(
			serviceID,
			space,
			&blobcmds.AcceptArguments{
				Blob: blobcmds.Blob{
					Digest: blobDigest,
					Size:   blobSize,
				},
				Put: promise.AwaitOK{Task: httpPutTask},
			},
			invocation.WithAudience(storageProvider.DID()),
		),
	)(t)
}

func invokePDPAccept(
	t *testing.T,
	service ucan.Signer,
	storageProvider ucan.Signer,
	blobDigest multihash.Multihash,
) ucan.Invocation {
	return testutil.Must(
		pdpcmds.Accept.Invoke(
			storageProvider,
			storageProvider.DID(),
			&pdpcmds.AcceptArguments{
				Blob: blobDigest,
			},
			invocation.WithAudience(storageProvider.DID()),
		),
	)(t)
}

func executeAccept(
	t *testing.T,
	acceptInv ucan.Invocation,
	storageProvider ucan.Signer,
	space did.DID,
	blobDigest multihash.Multihash,
	pdpAcceptTask cid.Cid,
	pdpAcceptInv ucan.Invocation,
) (ucan.Receipt, ucan.Invocation) {
	locationClaim := testutil.Must(assertcmds.Location.Invoke(
		storageProvider,
		storageProvider.DID(),
		&assertcmds.LocationArguments{
			Space:   space,
			Content: blobDigest,
			Location: []commands.CborURL{
				commands.CborURL(*testutil.Must(url.Parse("https://storage.example/fetch/" + digestutil.Format(blobDigest)))(t)),
			},
		},
		invocation.WithNoExpiration(),
	))(t)

	acceptReceipt := testutil.Must(
		receipt.IssueOK(
			storageProvider,
			acceptInv.Task().Link(),
			&blobcmds.AcceptOK{
				Site: locationClaim.Link(),
				PDP:  promise.AwaitOK{Task: pdpAcceptTask},
			},
		),
	)(t)

	return acceptReceipt, locationClaim
}

func executePDPAccept(
	t *testing.T,
	pdpAcceptInv ucan.Invocation,
	storageProvider ucan.Signer,
	blobDigest multihash.Multihash,
) ucan.Receipt {
	// Create mock piece links and inclusion proof for testing
	// In a real system, these would come from the PDP aggregation service

	// Create mock data commitments (32 bytes each, representing Filecoin commitments)
	// These are arbitrary but valid for testing purposes
	aggregateCommitment := make([]byte, 32)
	for i := range aggregateCommitment {
		aggregateCommitment[i] = byte(i)
	}

	individualCommitment := make([]byte, 32)
	for i := range individualCommitment {
		individualCommitment[i] = byte(i + 1)
	}

	// Create piece digests with small unpadded sizes for testing
	aggregatePiece := testutil.Must(piece.FromCommitmentAndSize(aggregateCommitment, 1024))(t)
	individualPiece := testutil.Must(piece.FromCommitmentAndSize(individualCommitment, 512))(t)

	// Create a mock inclusion proof
	// For testing, we just need valid proof data structure
	inclusionProof := merkletree.ProofData{
		Index: 0,
		Path:  []merkletree.Node{},
	}

	return testutil.Must(
		receipt.IssueOK(
			storageProvider,
			pdpAcceptInv.Task().Link(),
			&pdpcmds.AcceptOK{
				Aggregate:      aggregatePiece.CID(),
				InclusionProof: inclusionProof,
				Piece:          individualPiece.CID(),
			},
		),
	)(t)
}

// BlobAddHandler returns a mock [server.Route] to handle [blobcmds.Add]
// invocations in a test. rcptIssued is called with each receipt that is issued
// along the way. If includePutReceipt is true, a successful http/put receipt is
// embedded in the response (simulating a blob that was already uploaded).
func BlobAddHandler(
	t *testing.T,
	serviceID ucan.Signer,
	rcptIssued func(task cid.Cid, ct ucan.Container),
	includePutReceipt bool,
) server.Route {
	storageProvider := testutil.Must(ed25519.Generate())(t)

	// TK: why?
	// random signer rather than the proper derived one
	//blobProvider, err := ed25519signer.FromSeed([]byte(blobDigest)[len(blobDigest)-32:])
	blobProvider := testutil.Must(ed25519.Generate())(t)

	return server.NewRoute(
		blobcmds.Add,
		func(req *binding.Request[*blobcmds.AddArguments], res *binding.Response[*blobcmds.AddOK]) error {
			args := req.Task().Arguments()
			inv := req.Invocation()
			space := req.Invocation().Subject()
			blobDigest := args.Blob.Digest
			blobSize := args.Blob.Size

			allocateArgs := blobcmds.AllocateArguments{
				Blob:  blobcmds.Blob{Digest: blobDigest, Size: blobSize},
				Cause: inv.Task().Link(),
			}
			allocateInv := testutil.Must(blobcmds.Allocate.Invoke(
				serviceID,
				space,
				&allocateArgs,
				invocation.WithAudience(storageProvider.DID()),
			))(t)

			allocateRcpt := executeAllocate(t, &allocateArgs, allocateInv, storageProvider, blobSize)
			rcptIssued(allocateRcpt.Ran(), container.New(container.WithReceipts(allocateRcpt)))

			httpPutInv := invokePut(
				t,
				blobProvider,
				blobDigest,
				blobSize,
				allocateInv.Task().Link(),
			)

			acceptInv := invokeAccept(
				t,
				serviceID,
				storageProvider,
				space,
				blobDigest,
				blobSize,
				httpPutInv.Task().Link(),
			)

			pdpAcceptInv := invokePDPAccept(
				t,
				serviceID,
				storageProvider,
				blobDigest,
			)

			pdpAcceptRcpt := executePDPAccept(
				t,
				pdpAcceptInv,
				storageProvider,
				blobDigest,
			)

			rcptIssued(pdpAcceptRcpt.Ran(), container.New(container.WithReceipts(pdpAcceptRcpt)))
			pdpAcceptTask := pdpAcceptInv.Task().Link()

			acceptRcpt, locationClaim := executeAccept(
				t,
				acceptInv,
				storageProvider,
				space,
				blobDigest,
				pdpAcceptTask,
				pdpAcceptInv,
			)

			rcptIssued(
				acceptRcpt.Ran(),
				container.New(
					container.WithReceipts(acceptRcpt),
					container.WithInvocations(locationClaim),
				),
			)

			rcpts := []ucan.Receipt{allocateRcpt}
			if includePutReceipt {
				putRcpt := testutil.Must(receipt.IssueOK(
					blobProvider,
					httpPutInv.Task().Link(),
					&httpcmds.PutOK{},
				))(t)
				rcpts = append(rcpts, putRcpt)
			}
			res.SetMetadata(
				container.New(
					container.WithInvocations(allocateInv, httpPutInv, acceptInv),
					container.WithReceipts(rcpts...),
				),
			)

			return res.SetSuccess(&blobcmds.AddOK{
				Site: promise.AwaitOK{Task: acceptInv.Task().Link()},
			})
		},
	)
}

// receiptsTransport is an [http.RoundTripper] (an [http.Client] transport) that
// serves known receipts directly rather than using the network.
type receiptsTransport struct {
	receiptsLk sync.RWMutex
	receipts   map[cid.Cid]ucan.Container // task -> container with receipt and (possibly) claims
}

var _ http.RoundTripper = (*receiptsTransport)(nil)

func (r *receiptsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	path := req.URL.Path
	task := path[10:]

	r.receiptsLk.RLock()
	ct, ok := r.receipts[cid.MustParse(task)]
	r.receiptsLk.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no receipt for invocation %s", task)
	}
	return transport.DefaultHTTPInboundCodec.Encode(ct)
}

// WithBlobAdd creates an [Option] that adds `space/blob/add` support to
// the server. NB: This takes over the receipts client entirely. Currently,
// different options can't cooperate to share a receipts client. That's
// solvable, but hasn't been necessary yet.
func WithBlobAdd(t *testing.T) Option {
	return withBlobAdd(t, false)
}

// WithBlobAddPutReceipt is like WithBlobAdd but includes a
// successful /http/put receipt in the /blob/add response, simulating a
// blob that was already uploaded by a previous attempt.
func WithBlobAddPutReceipt(t *testing.T) Option {
	return withBlobAdd(t, true)
}

func withBlobAdd(t *testing.T, includePutReceipt bool) Option {
	receiptsTrans := receiptsTransport{
		receipts: map[cid.Cid]ucan.Container{},
	}

	return ComposeOptions(
		WithServerRoutes(
			func(deps RouteDeps) server.Route {
				return BlobAddHandler(
					t,
					deps.ServiceID,
					func(task cid.Cid, ct ucan.Container) {
						receiptsTrans.receiptsLk.Lock()
						defer receiptsTrans.receiptsLk.Unlock()
						receiptsTrans.receipts[task] = ct
					},
					includePutReceipt,
				)
			},
			func(_ RouteDeps) server.Route {
				return server.NewRoute(
					ucancmds.Conclude,
					func(req *binding.Request[*ucancmds.ConcludeArguments], res *binding.Response[*ucancmds.ConcludeOK]) error {
						return res.SetSuccess(&ucancmds.ConcludeOK{})
					},
				)
			},
		),
		WithClientOptions(
			client.WithReceiptsClient(
				receiptclient.NewClient(
					helpers.Must(url.Parse("https://receipts.example/receipts")),
					receiptclient.WithHTTPClient(
						&http.Client{
							Transport: &receiptsTrans,
						},
					),
				),
			),
		),
	)
}

type BlobMap = bytemap.ByteMap[multihash.Multihash, []byte]

type BlobReceiver interface {
	ReceivedBlobs() BlobMap
}

// blobPutTransport is an [http.RoundTripper] (an [http.Client] transport) that
// accepts blob PUTs and remembers what was received.
type blobPutTransport struct {
	receivedBlobsLk sync.RWMutex
	receivedBlobs   BlobMap
}

var _ http.RoundTripper = (*blobPutTransport)(nil)
var _ BlobReceiver = (*blobPutTransport)(nil)

func (r *blobPutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	if len(url) < len(storageURLPrefix) || url[:len(storageURLPrefix)] != storageURLPrefix {
		return nil, fmt.Errorf("unexpected PUT URL: %s", req.URL)
	}
	digestString := url[len(storageURLPrefix):]
	digest, err := digestutil.Parse(digestString)
	if err != nil {
		return nil, fmt.Errorf("decoding multihash: %w", err)
	}

	blob, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("reading blob from request: %w", err)
	}
	r.receivedBlobsLk.Lock()
	defer r.receivedBlobsLk.Unlock()
	r.receivedBlobs.Set(digest, blob)

	return &http.Response{
		StatusCode: 200,
	}, nil
}

func (r *blobPutTransport) ReceivedBlobs() BlobMap {
	return r.receivedBlobs
}

func ReceivedBlobs(putClient *http.Client) BlobMap {
	transport, ok := putClient.Transport.(BlobReceiver)
	if !ok {
		panic("The client isn't tracking PUTs. Create a client with NewPutClient() to use ReceivedBlobs().")
	}
	return transport.ReceivedBlobs()
}

// NewPutClient creates a new mock [http.Client] that accepts and tracks any PUT
// request, without making an actual network request.
func NewPutClient() *http.Client {
	return &http.Client{
		Transport: &blobPutTransport{
			receivedBlobs: bytemap.NewByteMap[multihash.Multihash, []byte](-1),
		},
	}
}
