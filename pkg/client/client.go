package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"

	rclient "github.com/fil-forge/go-ucanto/client/retrieval"
	receiptclient "github.com/fil-forge/guppy/pkg/receipt"
	"github.com/fil-forge/guppy/pkg/tokenstore"
	"github.com/fil-forge/ucantone/client"
	"github.com/fil-forge/ucantone/did"
	edm "github.com/fil-forge/ucantone/errors/datamodel"
	"github.com/fil-forge/ucantone/execution"
	"github.com/fil-forge/ucantone/ipld/datamodel"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	log    = logging.Logger("github.com/fil-forge/guppy/pkg/client")
	tracer = otel.Tracer("github.com/fil-forge/guppy/pkg/client")
)

type Client struct {
	signer         ucan.Signer
	serviceID      did.DID
	httpClient     *http.Client
	ucanClient     *client.HTTPClient
	ucanOpts       []client.HTTPOption
	receiptsClient *receiptclient.Client
	tokenStore     tokenstore.Store
	retrievalOpts  []rclient.Option
}

func New(signer ucan.Signer, serviceID did.DID, serviceURL url.URL, options ...Option) (*Client, error) {
	c := Client{
		signer:         signer,
		serviceID:      serviceID,
		receiptsClient: DefaultReceiptsClient,
	}

	for _, opt := range options {
		if err := opt(&c); err != nil {
			return nil, err
		}
	}

	// Create a default memory store if none provided
	if c.tokenStore == nil {
		c.tokenStore = tokenstore.NewMemStore()
	}

	ucanClient, err := client.NewHTTP(&serviceURL, c.ucanOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating UCAN client: %w", err)
	}
	c.ucanClient = ucanClient

	return &c, nil
}

// DID returns the DID of the agent.
func (c *Client) DID() did.DID {
	return c.signer.DID()
}

// Issuer returns the issuing signer of the agent.
func (c *Client) Issuer() ucan.Signer {
	return c.signer
}

// ProofChain recursively builds a proof chain of delegations from the given
// audience to the given subject for the specified command. It returns the
// list of delegations and their corresponding links in the order required for
// invocation. i.e. starting from the root Delegation (issued by the Subject),
// in strict sequence where the aud of the previous Delegation matches the iss
// of the next Delegation.
func (c *Client) ProofChain(ctx context.Context, aud did.DID, cmd ucan.Command, sub did.DID) ([]ucan.Delegation, []cid.Cid, error) {
	return c.tokenStore.ProofChain(ctx, aud, cmd, sub)
}

// ProofAttestations returns a list of attestations for proofs that need them.
// i.e. if a proof is signed with a non-standard signature this function will
// fetch an attestation for it, and fail if it cannot. The authority parameter
// is the DID of the service we trust to be issuing attestations.
func (c *Client) ProofAttestations(ctx context.Context, proofs []ucan.Delegation, authority did.DID) ([]ucan.Invocation, error) {
	return c.tokenStore.ProofAttestations(ctx, proofs, authority)
}

// AddProofs adds the given delegations to the client's token store.
func (c *Client) AddProofs(ctx context.Context, delegations ...ucan.Delegation) error {
	return c.tokenStore.AddDelegations(ctx, delegations...)
}

// Reset clears all tokens from the token store.
func (c *Client) Reset(ctx context.Context) error {
	return c.tokenStore.Reset(ctx)
}

// Execute sends the given invocation using the provided client and decodes the
// response into the specified type.
func Execute[T cbg.CBORUnmarshaler](
	ctx context.Context,
	client *client.HTTPClient,
	inv ucan.Invocation,
	options ...execution.RequestOption,
) (T, ucan.Receipt, ucan.Container, error) {
	fields := []zap.Field{
		zap.Stringer("issuer", inv.Issuer()),
		zap.Stringer("subject", inv.Subject()),
		zap.Stringer("command", inv.Command()),
		zap.Stringer("task", inv.Task().Link()),
		zap.Object("arguments", RawMap(inv.ArgumentsBytes())),
	}
	if inv.Audience().Defined() {
		fields = append(fields, zap.Stringer("audience", inv.Audience()))
	}
	if len(inv.MetadataBytes()) > 0 {
		fields = append(fields, zap.Object("metadata", RawMap(inv.MetadataBytes())))
	}
	if len(inv.Proofs()) > 0 {
		fields = append(fields, zap.Stringers("proofs", inv.Proofs()))
	}
	log := log.With(zap.Dict("invocation", fields...))
	log.Debug("executing invocation")

	var zero T
	resp, err := client.Execute(execution.NewRequest(ctx, inv, options...))
	if err != nil {
		log.Error("failed to execute invocation", zap.Error(err))
		return zero, nil, nil, fmt.Errorf("executing invocation: %w", err)
	}

	rcpt := resp.Receipt()
	o, x := rcpt.Out().Unpack()

	log = log.With(zap.Dict(
		"receipt",
		zap.Stringer("ran", rcpt.Ran()),
		zap.Dict("out", zap.Object("ok", RawMap(o)), zap.Object("err", RawMap(x))),
	))

	if rcpt.Out().IsErr() {
		log.Error("failed execution")
		var model edm.ErrorModel
		if err := model.UnmarshalCBOR(bytes.NewReader(x)); err != nil {
			log.Error("failed to unmarshal execution failure", zap.Error(err))
			return zero, nil, nil, fmt.Errorf("executing invocation")
		}
		return zero, nil, nil, fmt.Errorf("executing invocation: %w", model)
	}
	log.Debug("successful execution")

	// if ok is a pointer type, allocate the underlying value so
	// UnmarshalCBOR has a non-nil pointer to write into.
	var ok T
	typ := reflect.TypeOf(ok)
	if typ.Kind() == reflect.Ptr {
		ok = reflect.New(typ.Elem()).Interface().(T)
	}
	if err := ok.UnmarshalCBOR(bytes.NewReader(o)); err != nil {
		log.Error("failed to unmarshal invocation response", zap.Error(err))
		return zero, nil, nil, fmt.Errorf("unmarshaling invocation response: %w", err)
	}
	return ok, rcpt, resp.Metadata(), nil
}

// RawMap is a [zapcore.ObjectMarshaler] that decodes the given bytes as a
// CBOR-encoded IPLD map and logs its keys and values.
type RawMap []byte

func (rm RawMap) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if len(rm) == 0 {
		return nil
	}
	var m datamodel.Map
	if err := m.UnmarshalCBOR(bytes.NewReader(rm)); err != nil {
		return err
	}
	for k, v := range m {
		err := enc.AddReflected(k, v)
		if err != nil {
			return err
		}
	}
	return nil
}
