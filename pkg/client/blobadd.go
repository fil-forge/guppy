package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/fil-forge/guppy/internal/ctxutil"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	blobcmds "github.com/fil-forge/libforge/commands/blob"
	httpcmds "github.com/fil-forge/libforge/commands/http"
	ucancmds "github.com/fil-forge/libforge/commands/ucan"
	receipt_client "github.com/fil-forge/libforge/receipt"
	ucanlib "github.com/fil-forge/libforge/ucan"
	"github.com/fil-forge/ucantone/did"
	edm "github.com/fil-forge/ucantone/errors/datamodel"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ipld"
	"github.com/fil-forge/ucantone/ipld/datamodel"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/delegation"
	"github.com/fil-forge/ucantone/ucan/delegation/policy"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/fil-forge/ucantone/ucan/receipt"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// BlobAddOption configures options for [BlobAdd].
type BlobAddOption func(*BlobAddConfig)

// BlobAddConfig holds configuration for [BlobAdd].
type BlobAddConfig struct {
	PutClient          *http.Client
	PrecomputedDigest  multihash.Multihash
	PrecomputedSizePtr *uint64
	ProgressFn         func(uploaded int64)
}

// NewBlobAddConfig creates a new [BlobAddConfig] with the given options.
func NewBlobAddConfig(options ...BlobAddOption) *BlobAddConfig {
	cfg := &BlobAddConfig{
		PutClient: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
	for _, opt := range options {
		opt(cfg)
	}
	return cfg
}

// WithPutClient configures the HTTP client to use for uploading blobs.
func WithPutClient(client *http.Client) BlobAddOption {
	return func(cfg *BlobAddConfig) {
		cfg.PutClient = client
	}
}

// WithPrecomputedDigest supplies a previously computed digest/size so we can skip re-hashing.
func WithPrecomputedDigest(d multihash.Multihash, size uint64) BlobAddOption {
	return func(cfg *BlobAddConfig) {
		cfg.PrecomputedDigest = d
		cfg.PrecomputedSizePtr = &size
	}
}

// WithPutProgress registers a callback to receive upload progress for the HTTP PUT.
// The callback is best-effort and may be invoked frequently.
func WithPutProgress(progressFn func(uploaded int64)) BlobAddOption {
	return func(cfg *BlobAddConfig) {
		cfg.ProgressFn = progressFn
	}
}

type AddedBlob struct {
	Digest    multihash.Multihash
	Size      uint64
	Location  ucan.Invocation
	PDPAccept ucan.Invocation
}

// BlobAdd adds a blob to the service. The issuer needs proof of
// `/blob/add` delegated capability.
//
// Required delegated capability proofs: `/blob/add`
//
// The `space` is the resource the invocation applies to. It is typically the
// DID of a space.
//
// The `content` is the blob content to be added.
//
// The `proofs` are delegation proofs to use in addition to those in the client.
// They won't be saved in the client, only used for this invocation.
//
// Returns the multihash of the added blob and the location commitment that contains details about where the
// blob can be located, or an error if something went wrong.
func (c *Client) BlobAdd(ctx context.Context, content io.Reader, space did.DID, options ...BlobAddOption) (AddedBlob, error) {
	ctx, span := tracer.Start(ctx, "blob-add", trace.WithAttributes(
		attribute.String("space", space.String()),
	))
	defer span.End()

	// Configure options
	cfg := NewBlobAddConfig(options...)

	putClient := cfg.PutClient
	contentReader := content
	contentHash := cfg.PrecomputedDigest
	contentSizePtr := cfg.PrecomputedSizePtr
	needsHash := contentSizePtr == nil || contentHash == nil || len(contentHash) == 0
	start := time.Now()
	log.Infow("blob adding", "space", space, "has_digest", !needsHash)
	defer func() {
		log.Infow("blob added", "space", space, "has_digest", !needsHash, "duration", time.Since(start))
	}()
	if needsHash {
		ctx, readSpan := tracer.Start(ctx, "read-content")
		contentBytes, err := io.ReadAll(content)
		if err != nil {
			readSpan.End()
			return AddedBlob{}, fmt.Errorf("reading content: %w", err)
		}
		readSpan.SetAttributes(attribute.Int("content-size", len(contentBytes)))
		readSpan.End()
		_, hashSpan := tracer.Start(ctx, "hash-content", trace.WithAttributes(
			attribute.Int("content-size", len(contentBytes)),
		))
		contentHash, err = multihash.Sum(contentBytes, multihash.SHA2_256, -1)
		if err != nil {
			hashSpan.End()
			return AddedBlob{}, fmt.Errorf("computing content multihash: %w", err)
		}
		hashSpan.End()
		contentReader = bytes.NewReader(contentBytes)
		contentSize := uint64(len(contentBytes))
		contentSizePtr = &contentSize
	}

	if cfg.ProgressFn != nil {
		contentReader = &progressReader{
			r:        contentReader,
			total:    *contentSizePtr,
			progress: cfg.ProgressFn,
		}
	}

	proofs, proofLinks, err := c.ProofChain(ctx, c.signer.DID(), blobcmds.Add.Command, space)
	if err != nil {
		return AddedBlob{}, fmt.Errorf("building proof chain: %w", err)
	}
	attestations, err := c.ProofAttestations(ctx, proofs, c.serviceID)
	if err != nil {
		return AddedBlob{}, fmt.Errorf("fetching proof attestations: %w", err)
	}

	dlgPolicy, err := policy.Build(
		policy.Equal(".blob.digest", []byte(contentHash)),
		policy.Equal(".blob.size", int64(*contentSizePtr)),
	)
	if err != nil {
		return AddedBlob{}, fmt.Errorf("building delegation policy: %w", err)
	}

	allocDlg, allocProofs, allocAttestations, err := delegateWithProofs(
		ctx,
		c.signer,
		c.serviceID,
		space,
		blobcmds.Allocate.Command,
		dlgPolicy,
		c,
		c.serviceID,
	)
	if err != nil {
		return AddedBlob{}, fmt.Errorf("delegating /blob/allocate: %w", err)
	}

	accDlg, accProofs, accAttestations, err := delegateWithProofs(
		ctx,
		c.signer,
		c.serviceID,
		space,
		blobcmds.Accept.Command,
		dlgPolicy,
		c,
		c.serviceID,
	)
	if err != nil {
		return AddedBlob{}, fmt.Errorf("delegating /blob/accept: %w", err)
	}

	inv, err := blobcmds.Add.Invoke(
		c.signer,
		space,
		&blobcmds.AddArguments{
			Blob: blobcmds.Blob{
				Digest: contentHash,
				Size:   *contentSizePtr,
			},
		},
		invocation.WithAudience(c.serviceID),
		invocation.WithProofs(proofLinks...),
	)
	if err != nil {
		return AddedBlob{}, fmt.Errorf("generating invocation: %w", err)
	}

	addOK, _, meta, err := Execute[*blobcmds.AddOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithDelegations(proofs...),
		execution.WithInvocations(attestations...),
		// add allocate/accept delegations
		execution.WithDelegations(allocDlg, accDlg),
		// ...and their proofs and attestations
		execution.WithDelegations(allocProofs...),
		execution.WithInvocations(allocAttestations...),
		execution.WithDelegations(accProofs...),
		execution.WithInvocations(accAttestations...),
	)
	if err != nil {
		return AddedBlob{}, fmt.Errorf("executing blob add: %w", err)
	}

	accInv, err := findInvocation(addOK.Site.Task, meta.Invocations())
	if err != nil {
		return AddedBlob{}, fmt.Errorf("finding /blob/accept invocation: %w", err)
	}
	var accArgs blobcmds.AcceptArguments
	if err := accArgs.UnmarshalCBOR(bytes.NewReader(accInv.ArgumentsBytes())); err != nil {
		return AddedBlob{}, fmt.Errorf("unmarshaling /blob/accept arguments: %w", err)
	}

	putInv, err := findInvocation(accArgs.Put.Task, meta.Invocations())
	if err != nil {
		return AddedBlob{}, fmt.Errorf("finding /http/put invocation: %w", err)
	}
	var putArgs httpcmds.PutArguments
	if err := putArgs.UnmarshalCBOR(bytes.NewReader(putInv.ArgumentsBytes())); err != nil {
		return AddedBlob{}, fmt.Errorf("unmarshaling /http/put arguments: %w", err)
	}
	putRcpt := maybeFindReceipt(accArgs.Put.Task, meta.Receipts())

	allocInv, err := findInvocation(putArgs.Destination.Task, meta.Invocations())
	if err != nil {
		return AddedBlob{}, fmt.Errorf("finding /blob/allocate invocation: %w", err)
	}
	var allocArgs blobcmds.AllocateArguments
	if err := allocArgs.UnmarshalCBOR(bytes.NewReader(allocInv.ArgumentsBytes())); err != nil {
		return AddedBlob{}, fmt.Errorf("unmarshaling /blob/allocate arguments: %w", err)
	}
	allocRcpt, err := findReceipt(putArgs.Destination.Task, meta.Receipts())
	if err != nil {
		return AddedBlob{}, fmt.Errorf("finding /blob/allocate receipt: %w", err)
	}
	o, x := allocRcpt.Out().Unpack()
	if allocRcpt.Out().IsErr() {
		var model edm.ErrorModel
		if err := model.UnmarshalCBOR(bytes.NewReader(x)); err != nil {
			log.Errorw("failed to unmarshal allocation execution failure", "error", err)
			return AddedBlob{}, fmt.Errorf("executing invocation")
		}
		return AddedBlob{}, fmt.Errorf("failure in allocation receipt: %w", model)
	}
	var allocOK blobcmds.AllocateOK
	if err := allocOK.UnmarshalCBOR(bytes.NewReader(o)); err != nil {
		return AddedBlob{}, fmt.Errorf("unmarshaling allocation receipt output: %w", err)
	}

	putSuccess := putRcpt != nil && putRcpt.Out().IsOK()

	// only perform HTTP PUT if we have an address AND we haven't received a
	// success receipt for the `/http/put` task (which means the upload has
	// already been completed by a previous invocation attempt)
	if allocOK.Address != nil && !putSuccess {
		if err := putBlob(ctx, putClient, allocOK.Address.URL.URL(), allocOK.Address.Headers, contentReader); err != nil {
			return AddedBlob{}, fmt.Errorf("putting blob: %w", err)
		}
	}

	// invoke `/ucan/conclude` with `/http/put` receipt
	if !putSuccess {
		if err := c.sendPutReceipt(ctx, putInv); err != nil {
			return AddedBlob{}, fmt.Errorf("sending put receipt: %w", err)
		}
	}

	accRcpt, accMeta, err := c.receiptsClient.Poll(ctx, accInv.Task().Link(), receipt_client.WithRetries(5))
	if err != nil {
		return AddedBlob{}, fmt.Errorf("polling accept receipt: %w", err)
	}

	o, x = accRcpt.Out().Unpack()
	if accRcpt.Out().IsErr() {
		var model edm.ErrorModel
		if err := model.UnmarshalCBOR(bytes.NewReader(x)); err != nil {
			log.Errorw("failed to unmarshal accept execution failure", "error", err)
			return AddedBlob{}, fmt.Errorf("executing invocation")
		}
		return AddedBlob{}, fmt.Errorf("failure in accept receipt: %w", model)
	}

	var accOK blobcmds.AcceptOK
	if err := accOK.UnmarshalCBOR(bytes.NewReader(o)); err != nil {
		return AddedBlob{}, fmt.Errorf("unmarshaling accept receipt output: %w", err)
	}

	var locationCommitment ucan.Invocation
	var pdpAcceptInv ucan.Invocation
	for _, inv := range accMeta.Invocations() {
		switch inv.Command() {
		case assertcmds.Location.Command:
			locationCommitment = inv
		// TODO: use PDP commands when landed https://github.com/fil-forge/libforge/pull/28
		default:
			if inv.Command().String() == "/pdp/accept" {
				pdpAcceptInv = inv
			}
		}
	}
	if locationCommitment == nil {
		return AddedBlob{}, fmt.Errorf("blob accept receipt missing location commitment invocation")
	}
	if pdpAcceptInv == nil {
		return AddedBlob{}, fmt.Errorf("blob accept receipt missing PDP accept invocation")
	}

	return AddedBlob{
		Digest:    contentHash,
		Size:      *contentSizePtr,
		Location:  locationCommitment,
		PDPAccept: pdpAcceptInv,
	}, nil
}

func putBlob(ctx context.Context, client *http.Client, url *url.URL, headers map[string]string, body io.Reader) error {
	start := time.Now()
	log.Infow("putting blob", "destination", url.String())
	defer func() {
		log.Infow("put blob", "destination", url.String(), "duration", time.Since(start))
	}()

	sw := newStallWarnReader(body, url.String(), 30*time.Second)
	defer sw.stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url.String(), sw)
	if err != nil {
		return fmt.Errorf("creating upload request: %w", ctxutil.EnrichWithCause(err, ctx))
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("uploading blob: %w", ctxutil.EnrichWithCause(err, ctx))
	}
	if err := resp.Body.Close(); err != nil {
		log.Warnf("closing upload response body: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("uploading blob: %s", resp.Status)
	}

	return nil
}

func (c *Client) sendPutReceipt(ctx context.Context, putInv ucan.Invocation) error {
	var putMeta datamodel.Map
	if err := putMeta.UnmarshalCBOR(bytes.NewReader(putInv.MetadataBytes())); err != nil {
		return fmt.Errorf("unmarshaling /http/put invocation metadata: %w", err)
	}
	keysMap, ok := putMeta["keys"].(ipld.Map)
	if !ok {
		return fmt.Errorf("invalid put metadata, missing 'keys' field")
	}
	did, ok := keysMap["id"].(string)
	if !ok {
		return fmt.Errorf("invalid put metadata, missing 'id' field in 'keys'")
	}
	keysKeysMap, ok := keysMap["keys"].(ipld.Map)
	if !ok {
		return fmt.Errorf("invalid put metadata, missing 'keys' field in 'keys'")
	}
	keyBytes, ok := keysKeysMap[did].([]byte)
	if !ok {
		return fmt.Errorf("invalid put metadata, missing key for %s", did)
	}
	signer, err := ed25519.Decode(keyBytes)
	if err != nil {
		return fmt.Errorf("decoding key for %q: %w", did, err)
	}
	putRcpt, err := receipt.IssueOK(signer, putInv.Task().Link(), &httpcmds.PutOK{}, receipt.WithIssuedAt(ucan.Now()))
	if err != nil {
		return fmt.Errorf("generating receipt: %w", err)
	}

	inv, err := ucancmds.Conclude.Invoke(
		c.signer,
		c.signer.DID(),
		&ucancmds.ConcludeArguments{
			Receipt: putRcpt.Link(),
		},
		invocation.WithAudience(c.serviceID),
	)
	if err != nil {
		return fmt.Errorf("generating invocation: %w", err)
	}

	_, rcpt, _, err := Execute[*ucancmds.ConcludeOK](
		ctx,
		c.ucanClient,
		inv,
		execution.WithReceipts(putRcpt),
	)
	if err != nil {
		return fmt.Errorf("executing invocation: %w", ctxutil.EnrichWithCause(err, ctx))
	}
	if rcpt.Out().IsErr() {
		_, x := rcpt.Out().Unpack()
		var model edm.ErrorModel
		if err := model.UnmarshalCBOR(bytes.NewReader(x)); err != nil {
			return fmt.Errorf("conclude failed with unknown error: %w", err)
		}
		return fmt.Errorf("conclude failed: %w", model)
	}
	return nil
}

type progressReader struct {
	r         io.Reader
	total     uint64
	progress  func(uploaded int64)
	readSoFar int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.readSoFar += int64(n)
		p.progress(p.readSoFar)
	}
	return n, err
}

type stallWarnReader struct {
	r          io.Reader
	name       string
	threshold  time.Duration
	timer      *time.Timer
	stallStart time.Time
	warned     bool
}

var _ io.Reader = (*stallWarnReader)(nil)

// newStallWarnReader wraps an io.Reader and logs a warning if no data is read
// from it before [threshold], and again every [threshold] until some data is
// read. The timer begins when the stallWarnReader is created, and is reset
// after each successful read, until `stop()` is called.
func newStallWarnReader(r io.Reader, name string, threshold time.Duration) *stallWarnReader {
	sw := &stallWarnReader{
		r:         r,
		name:      name,
		threshold: threshold,
	}
	sw.stallStart = time.Now()
	sw.timer = time.AfterFunc(threshold, sw.warn)
	return sw
}

func (sw *stallWarnReader) warn() {
	sw.warned = true
	elapsed := time.Since(sw.stallStart)
	log.Warnw("blob upload stalled", "destination", sw.name, "stalled_for", elapsed)
	sw.timer.Reset(sw.threshold)
}

func (sw *stallWarnReader) Read(p []byte) (int, error) {
	sw.timer.Stop()
	if sw.warned {
		sw.warned = false
		elapsed := time.Since(sw.stallStart)
		log.Warnw("blob upload stall ended", "destination", sw.name, "stalled_for", elapsed)
	}
	n, err := sw.r.Read(p)
	sw.stallStart = time.Now()
	sw.timer.Reset(sw.threshold)
	return n, err
}

func (sw *stallWarnReader) stop() {
	sw.timer.Stop()
}

func delegateWithProofs(
	ctx context.Context,
	issuer ucan.Signer,
	audience did.DID,
	subject did.DID,
	command ucan.Command,
	pol ucan.Policy,
	proofStore ucanlib.ProofStore,
	attestationAuthority did.DID,
) (ucan.Delegation, []ucan.Delegation, []ucan.Invocation, error) {
	dlg, err := delegation.Delegate(
		issuer,
		audience,
		subject,
		command,
		delegation.WithPolicy(pol),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("delegating: %w", err)
	}
	proofs, _, err := proofStore.ProofChain(ctx, issuer.DID(), command, subject)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("building proof chain: %w", err)
	}
	attestations, err := proofStore.ProofAttestations(ctx, proofs, attestationAuthority)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetching proof attestations: %w", err)
	}
	return dlg, proofs, attestations, nil
}

func findInvocation(task cid.Cid, invocations []ucan.Invocation) (ucan.Invocation, error) {
	for _, inv := range invocations {
		if inv.Task().Link() == task {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("missing invocation for task: %s", task)
}

func findReceipt(task cid.Cid, receipts []ucan.Receipt) (ucan.Receipt, error) {
	for _, rcpt := range receipts {
		if rcpt.Ran() == task {
			return rcpt, nil
		}
	}
	return nil, fmt.Errorf("missing receipt for task: %s", task)
}

func maybeFindReceipt(task cid.Cid, receipts []ucan.Receipt) ucan.Receipt {
	for _, rcpt := range receipts {
		if rcpt.Ran() == task {
			return rcpt
		}
	}
	return nil
}
