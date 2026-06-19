package testutil

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/preparation/storacha"
	"github.com/fil-forge/libforge/testutil"
	uclient "github.com/fil-forge/ucantone/client"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/multikey/ed25519"
	"github.com/fil-forge/ucantone/server"
	"github.com/fil-forge/ucantone/ucan"
)

type clientServerConfig struct {
	serverRoutes  []RouteBuilderFunc
	serverOptions []server.HTTPOption
	clientOptions []client.Option
}

type RouteDeps struct {
	ServiceID ucan.Issuer
}

type RouteBuilderFunc func(RouteDeps) server.Route

type Option func(*clientServerConfig)

func WithServerRoutes(routes ...RouteBuilderFunc) Option {
	return func(c *clientServerConfig) {
		c.serverRoutes = append(c.serverRoutes, routes...)
	}
}

// WithServerOptions appends options to the server configuration.
func WithServerOptions(opts ...server.HTTPOption) Option {
	return func(c *clientServerConfig) {
		c.serverOptions = append(c.serverOptions, opts...)
	}
}

// WithClientOptions appends options to the client configuration.
func WithClientOptions(opts ...client.Option) Option {
	return func(c *clientServerConfig) {
		c.clientOptions = append(c.clientOptions, opts...)
	}
}

// Client creates an entire [client.Client] with a connection to an in-process
// server, each configured with the given options.
func Client(t *testing.T, options ...Option) (*client.Client, error) {
	config := &clientServerConfig{}
	for _, opt := range options {
		opt(config)
	}

	issuer := testutil.Must(ed25519.GenerateIssuer())(t)

	serviceID, srv := NewTestServer(t, config.serverOptions...)
	deps := RouteDeps{ServiceID: issuer}
	for _, routeBuilder := range config.serverRoutes {
		route := routeBuilder(deps)
		srv.Handle(route.Command, route.Handler)
	}
	// placeholder - client is attached directly via HTTP transport
	serviceURL := testutil.Must(url.Parse("http://localhost"))(t)

	config.clientOptions = append(
		config.clientOptions,
		client.WithUCANClientOptions(
			uclient.WithHTTPClient(&http.Client{Transport: srv}),
		),
	)
	return client.New(issuer, serviceID, *serviceURL, config.clientOptions...)
}

// ComposeOptions combines multiple options into one. It's written generically
// so that it might someday move somewhere more generic, but so far it's only
// being used here anyhow.
func ComposeOptions[C any](opts ...func(C)) func(C) {
	return func(c C) {
		for _, opt := range opts {
			opt(c)
		}
	}
}

// ClientWithCustomPut is a [client.Client] that uses a custom client for PUT
// requests from [SpaceBlobAdd].
type ClientWithCustomPut struct {
	*client.Client
	PutClient *http.Client
}

var _ storacha.Client = (*ClientWithCustomPut)(nil)

func (c *ClientWithCustomPut) BlobAdd(ctx context.Context, content io.Reader, space did.DID, options ...client.BlobAddOption) (client.AddedBlob, error) {
	return c.Client.BlobAdd(ctx, content, space, append(options, client.WithPutClient(c.PutClient))...)
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T {
	return &v
}
