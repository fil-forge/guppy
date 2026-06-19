package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/presets"
	"github.com/fil-forge/guppy/pkg/tokenstore"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/multikey"
	"github.com/fil-forge/ucantone/multikey/ed25519"
	"github.com/fil-forge/ucantone/ucan/container"
)

func main() {
	// private key to sign invocation UCAN with
	keybytes, _ := os.ReadFile("path/to/private.key")
	agentSigner, _ := ed25519.FromRaw(keybytes)
	agent := multikey.KeyIssuer(agentSigner)

	// UCAN proof that signer can list uploads in this space (a delegation chain)
	prfContainerBytes, _ := os.ReadFile("path/to/proof.ucan")
	prfContainer, _ := container.Decode(prfContainerBytes)

	// space to list uploads from
	space, _ := did.Parse("did:key:z6MkwDuRThQcyWjqNsK54yKAmzfsiH6BTkASyiucThMtHt1y")

	tokenStore := tokenstore.NewMemStore()
	tokenStore.AddDelegations(context.Background(), prfContainer.Delegations()...)

	c, _ := client.New(
		agent,
		presets.DefaultNetwork.UploadID,
		presets.DefaultNetwork.UploadURL,
		client.WithTokenStore(tokenStore),
	)

	listOK, _ := c.UploadList(
		context.Background(),
		space,
		uploadcmds.ListArguments{},
	)

	for _, r := range listOK.Results {
		fmt.Printf("%s\n", r.Root)
	}
}
