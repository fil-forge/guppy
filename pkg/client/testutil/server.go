package testutil

import (
	"testing"

	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/principal/signer"
	"github.com/fil-forge/ucantone/server"
)

// NewTestServer creates a new Ucanto server. It accepts `server.HTTPOption`s to
// configure the server.
//
// The server generates its own service principal. It has a `did:web:` DID for
// realism and readability in errors and failures.
func NewTestServer(t *testing.T, options ...server.HTTPOption) (did.DID, *server.HTTPServer) {
	servicePrincipal := testutil.Must(signer.Wrap(
		testutil.Must(ed25519.Generate())(t),
		testutil.Must(did.Parse("did:web:storage.example.com"))(t),
	))(t)

	return servicePrincipal.DID(), server.NewHTTP(servicePrincipal, options...)
}
