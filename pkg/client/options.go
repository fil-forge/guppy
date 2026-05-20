package client

import (
	"net/http"

	rclient "github.com/fil-forge/go-ucanto/client/retrieval"
	"github.com/fil-forge/guppy/pkg/receipt"
	"github.com/fil-forge/guppy/pkg/tokenstore"
)

// Option is an option configuring a Client.
type Option func(c *Client) error

// WithHTTPClient configures the HTTP client for the client to use. If one is
// not provided, the default HTTP client will be used.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) error {
		c.httpClient = client
		return nil
	}
}

// WithReceiptsClient configures the client to use for fetching receipts.
func WithReceiptsClient(receiptsClient *receipt.Client) Option {
	return func(c *Client) error {
		c.receiptsClient = receiptsClient
		return nil
	}
}

// WithTokenStore configures the token store for the client to use. If one is not
// provided, a new memory store will be created.
func WithTokenStore(store tokenstore.Store) Option {
	return func(c *Client) error {
		c.tokenStore = store
		return nil
	}
}

func WithRetrievalOptions(retrievalOpts ...rclient.Option) Option {
	return func(c *Client) error {
		c.retrievalOpts = append(c.retrievalOpts, retrievalOpts...)
		return nil
	}
}
