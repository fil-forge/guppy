package main

import (
	"context"
	"fmt"
	"os"

	uploadcap "github.com/fil-forge/go-libstoracha/capabilities/upload"
	"github.com/fil-forge/go-ucanto/did"
	"github.com/fil-forge/go-ucanto/principal/ed25519/signer"
	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/delegation"
	"github.com/fil-forge/guppy/pkg/tokenstore"
)

func main() {
	// private key to sign invocation UCAN with
	keybytes, _ := os.ReadFile("path/to/private.key")
	signer, _ := signer.FromRaw(keybytes)

	// UCAN proof that signer can list uploads in this space (a delegation chain)
	prfbytes, _ := os.ReadFile("path/to/proof.ucan")
	proof, _ := delegation.ExtractProof(prfbytes)

	// space to list uploads from
	space, _ := did.Parse("did:key:z6MkwDuRThQcyWjqNsK54yKAmzfsiH6BTkASyiucThMtHt1y")

	tokenStore := tokenstore.NewMemStore()
	tokenStore.AddDelegations(proof)

	// nil uses the default connection to the Storacha network
	c, _ := client.New(
		signer,
		client.WithTokenStore(tokenStore),
	)

	listOk, _ := c.UploadList(
		context.Background(),
		space,
		uploadcap.ListCaveats{},
	)

	for _, r := range listOk.Results {
		fmt.Printf("%s\n", r.Root)
	}
}
