// Package cmdutil provides utility functions specifically for the Guppy CLI.
package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/guppy/pkg/presets"
	"github.com/fil-forge/guppy/pkg/tokenstore"
	indexclient "github.com/fil-forge/indexing-service/pkg/client"
	"github.com/fil-forge/libforge/identity"
	receiptclient "github.com/fil-forge/libforge/receipt"
	utclient "github.com/fil-forge/ucantone/client"
	"github.com/fil-forge/ucantone/did"
	utd25519 "github.com/fil-forge/ucantone/principal/ed25519"
	utucan "github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/container"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// envSigner returns a signer from the environment variable GUPPY_PRIVATE_KEY,
// if any.
func envSigner() (utucan.Signer, error) {
	str := os.Getenv("GUPPY_PRIVATE_KEY") // use env var preferably
	if str == "" {
		return nil, nil // no signer in the environment
	}

	s, err := utd25519.Parse(str)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// TracedHTTPClient is an HTTP client with OpenTelemetry tracing and a guppy
// User-Agent header on all outbound requests.
var TracedHTTPClient = &http.Client{
	Transport: newUserAgentTransport(otelhttp.NewTransport(http.DefaultTransport)),
}

// MustGetClient creates a new client suitable for the CLI, using stored data,
// if any. The storePath should be a directory path where agent data will be stored.
// The networkCfg contains network settings from the config file.
func MustGetClient(cfg config.Config, options ...client.Option) *client.Client {
	return MustGetClientForNetwork(cfg, "", options...)
}

// MustGetClientForNetwork is like MustGetClient but allows specifying a network
// configuration by name (which may be empty). The networkCfg contains network
// settings from the config file, and flagName is the network name from CLI flag
// (takes precedence over config).
func MustGetClientForNetwork(cfg config.Config, flagName string, options ...client.Option) *client.Client {
	pem, err := os.ReadFile(cfg.Identity.KeyFile)
	if err != nil {
		log.Fatalf("reading key file: %s", err)
	}
	var agent utucan.Signer
	agent, err = identity.DecodeEd25519SignerFromPEM(pem)
	if err != nil {
		log.Fatalf("parsing key file: %s", err)
	}

	// Override the signer if the env var is set.
	if s, err := envSigner(); err != nil {
		log.Fatalf("parsing GUPPY_PRIVATE_KEY: %s", err)
	} else if s != nil {
		agent = s
	}

	store, err := tokenstore.NewFsStore(cfg.Repo.Dir)
	if err != nil {
		log.Fatalf("creating token store: %s", err)
	}

	network := MustGetNetworkConfig(cfg.Network, flagName)

	c, err := client.New(
		agent,
		network.UploadID,
		network.UploadURL,
		append(
			options,
			client.WithTokenStore(store),
			client.WithReceiptsClient(receiptclient.NewClient(&network.ReceiptsURL, receiptclient.WithHTTPClient(TracedHTTPClient))),
			client.WithUCANClientOptions(utclient.WithHTTPClient(TracedHTTPClient)),
		)...,
	)
	if err != nil {
		log.Fatalf("creating client: %s", err)
	}

	return c
}

// MustGetNetworkConfig resolves network configuration from config file and/or
// CLI flag. The flagName takes precedence over config values when set.
// Falls back to STORACHA_* env vars if config is empty.
func MustGetNetworkConfig(networkCfg config.NetworkConfig, flagName string) presets.NetworkConfig {
	// If config has values, use them (with flag name taking precedence for preset selection)
	if !networkCfg.IsEmpty() || flagName != "" {
		network, err := networkCfg.ToPresetConfig(flagName)
		if err != nil {
			log.Fatal(fmt.Errorf("getting network configuration: %w", err))
		}
		return network
	}

	// Fall back to preset-only behavior (handles STORACHA_* env vars)
	network, err := presets.GetNetworkConfig("")
	if err != nil {
		log.Fatal(fmt.Errorf("getting network configuration: %w", err))
	}
	return network
}

// MustGetIndexClient creates a new indexer client using the network configuration.
func MustGetIndexClient(networkCfg config.NetworkConfig) (*indexclient.Client, did.DID) {
	return MustGetIndexClientForNetwork(networkCfg, "")
}

// MustGetIndexClientForNetwork creates a new indexer client, allowing a CLI flag
// to override the network preset name.
func MustGetIndexClientForNetwork(networkCfg config.NetworkConfig, flagName string) (*indexclient.Client, did.DID) {
	network := MustGetNetworkConfig(networkCfg, flagName)

	client, err := indexclient.New(network.IndexerID, network.IndexerURL, indexclient.WithHTTPClient(TracedHTTPClient))
	if err != nil {
		log.Fatal(err)
	}

	return client, network.IndexerID
}

// AddProofsFromFile decodes a UCAN delegation container from the given path and
// adds its delegations to the client's token store.
func AddProofsFromFile(ctx context.Context, c *client.Client, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading proof file %q: %w", path, err)
	}
	ct, err := container.Decode(b)
	if err != nil {
		return fmt.Errorf("decoding proof container %q: %w", path, err)
	}
	if err := c.AddProofs(ctx, ct.Delegations()...); err != nil {
		return fmt.Errorf("adding proofs: %w", err)
	}
	return nil
}

// ParseSize parses a data size string with optional suffix (B, K, M, G).
// Accepts formats like: "1024", "512B", "100K", "50M", "2G". Digits with no
// suffix are interpreted as bytes. Returns the size in bytes.
func ParseSize(s string) (uint64, error) {
	if s == "" {
		return 0, errors.New("data size cannot be empty")
	}

	// Trim any whitespace
	s = strings.TrimSpace(s)

	// Check if it ends with a suffix
	var multiplier uint64 = 1
	var numStr string

	lastChar := strings.ToUpper(s[len(s)-1:])
	switch lastChar {
	case "B":
		multiplier = 1
		numStr = s[:len(s)-1]
	case "K":
		multiplier = 1024
		numStr = s[:len(s)-1]
	case "M":
		multiplier = 1024 * 1024
		numStr = s[:len(s)-1]
	case "G":
		multiplier = 1024 * 1024 * 1024
		numStr = s[:len(s)-1]
	default:
		// No suffix, assume bytes
		numStr = s
	}

	// Parse the numeric part
	num, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid shard size format: %w", err)
	}

	// Calculate the final size
	size := num * multiplier

	return size, nil
}

func NewHandledCliError(err error) HandledCliError {
	return HandledCliError{err}
}

// HandledCliError is an error which has already been presented to the user. If
// a HandledCliError is returned from a command, the process should exit with
// a non-zero exit code, but no further error message should be printed.
type HandledCliError struct {
	error
}

func (e HandledCliError) Unwrap() error {
	return e.error
}

// ResolveSpace resolves a space identifier, which can be either a DID or a name.
// If the identifier is a valid DID, it returns that DID directly.
// Otherwise, it looks up the space by name using the provided client.
// Returns an error if the name matches no spaces or multiple spaces.
func ResolveSpace(ctx context.Context, c *client.Client, identifier string) (did.DID, error) {
	// First, try to parse as a DID
	spaceDID, err := did.Parse(identifier)
	if err == nil {
		return spaceDID, nil
	}

	// Not a valid DID, try to look up by name
	space, err := c.SpaceNamed(ctx, identifier)
	if err != nil {
		var notFoundErr client.SpaceNotFoundError
		if errors.As(err, &notFoundErr) {
			return did.DID{}, fmt.Errorf("no space found with name %q", identifier)
		}
		var multipleErr client.MultipleSpacesFoundError
		if errors.As(err, &multipleErr) {
			return did.DID{}, fmt.Errorf("multiple spaces found with name %q; use DID to specify which one", identifier)
		}
		return did.DID{}, err
	}

	return space.DID(), nil
}
