package testutil

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	contentcmds "github.com/fil-forge/libforge/commands/content"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/libforge/ucan/retrieval"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/container"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
)

// RetrievalClientOption configures a retrieval client.
type RetrievalClientOption func(*retrievalClientConfig)

type retrievalClientConfig struct {
	skipHashValidation bool
}

// WithoutHashValidation disables server-side hash validation.
// This is useful for testing client-side validation.
func WithoutHashValidation() RetrievalClientOption {
	return func(cfg *retrievalClientConfig) {
		cfg.skipHashValidation = true
	}
}

// NewRetrievalClient creates an in-process retrieval server and returns an HTTP
// client that connects to it directly (without network I/O). The server handles
// `/content/retrieve` invocations, serving the requested byte range of testData.
// By default it validates that the capability digest (and the URL path hash, when
// present) match the actual data hash before serving. Use WithoutHashValidation()
// to disable that for testing client-side validation.
func NewRetrievalClient(t *testing.T, service ucan.Issuer, testData []byte, opts ...RetrievalClientOption) *http.Client {
	cfg := retrievalClientConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	srv := retrieval.NewServer(service)
	route := contentcmds.Retrieve.Route(
		func(req *binding.Request[*contentcmds.RetrieveArguments], res *binding.Response[*contentcmds.RetrieveOK]) error {
			args := req.Task().Arguments()

			if !cfg.skipHashValidation {
				actualHash, err := multihash.Sum(testData, multihash.SHA2_256, -1)
				if err != nil {
					return fmt.Errorf("hashing test data: %w", err)
				}
				// Capability digest must match the actual data.
				assert.Equal(t, actualHash, args.Blob.Digest, "capability digest should match actual data; test may be incorrect")

				// If the request carried a URL with a /blob/<hash> path, it must
				// match the capability digest too.
				if hcReq, ok := req.Metadata().(*retrieval.HTTPHeaderRequestContainer); ok && hcReq.URL != nil {
					urlPath := hcReq.URL.Path
					if len(urlPath) > len("/blob/") && urlPath[:len("/blob/")] == "/blob/" {
						urlHash, err := digestutil.Parse(urlPath[len("/blob/"):])
						if err != nil {
							return fmt.Errorf("parsing URL hash: %w", err)
						}
						assert.Equal(t, urlHash, args.Blob.Digest, "URL hash should match capability; test may be incorrect")
					}
				}
			}

			start := int(args.Range.Start)
			end := int(args.Range.End)
			if start < 0 || end >= len(testData) || start > end {
				return res.SetMetadata(&retrieval.HTTPHeaderResponseContainer{
					Container:  container.New(),
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{},
					Body:       io.NopCloser(bytes.NewReader(nil)),
				})
			}

			length := end - start + 1
			headers := http.Header{}
			headers.Set("Content-Length", fmt.Sprintf("%d", length))
			headers.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(testData)))

			if err := res.SetMetadata(&retrieval.HTTPHeaderResponseContainer{
				Container:  container.New(),
				StatusCode: http.StatusPartialContent,
				Header:     headers,
				Body:       io.NopCloser(bytes.NewReader(testData[start : end+1])),
			}); err != nil {
				return err
			}
			return res.SetSuccess(&contentcmds.RetrieveOK{})
		},
	)
	srv.Handle(route.Command, route.Handler)

	// The retrieval server is itself an http.RoundTripper, so an http.Client
	// using it as transport talks to it in-process.
	return &http.Client{Transport: srv}
}
