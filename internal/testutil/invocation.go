package testutil

import (
	"net/url"
	"testing"

	"github.com/fil-forge/libforge/commands"
	assertcmds "github.com/fil-forge/libforge/commands/assert"
	"github.com/fil-forge/libforge/digestutil"
	libtestutil "github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/ucan"
)

// RandomLocationInvocation returns a random `assert/location` commitment
// invocation, replacing go-libstoracha/testutil.RandomLocationInvocation which
// has no libforge equivalent.
func RandomLocationInvocation(t *testing.T) ucan.Invocation {
	t.Helper()
	signer := libtestutil.RandomSigner(t)
	space := libtestutil.RandomDID(t)
	digest := libtestutil.RandomMultihash(t)
	storageURL := libtestutil.Must(url.Parse("https://storage.example/blob/" + digestutil.Format(digest)))(t)
	return libtestutil.Must(assertcmds.Location.Invoke(
		signer,
		signer.DID(),
		&assertcmds.LocationArguments{
			Space:    space,
			Content:  digest,
			Location: []commands.CborURL{commands.CborURL(*storageURL)},
		},
	))(t)
}
