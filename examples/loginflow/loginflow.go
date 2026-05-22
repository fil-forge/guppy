package main

import (
	"context"
	"fmt"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/presets"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal/ed25519"
)

// Error handling omitted for brevity.

func main() {
	ctx := context.Background()

	agent, _ := ed25519.Generate()

	// space to list uploads from
	space, _ := did.Parse("did:key:z6MkwDuRThQcyWjqNsK54yKAmzfsiH6BTkASyiucThMtHt1y")

	// the account to log in as, which has access to the space
	account, _ := did.Parse("mailto:example.com:ucansam")

	c, _ := client.New(
		agent,
		presets.DefaultNetwork.UploadID,
		presets.DefaultNetwork.UploadURL,
	)

	// Kick off the login flow
	requestOK, _ := c.RequestAccess(ctx, account)

	// Start polling to see if the user has authenticated yet
	resultChan := c.PollClaim(ctx, requestOK)
	fmt.Println("Please click the link in your email to authenticate...")
	// Wait for the user to authenticate
	proofResult := <-resultChan
	proofs, _ := proofResult.Unpack()

	// Add the proofs to the client
	c.AddProofs(context.Background(), proofs...)

	listOk, _ := c.UploadList(
		context.Background(),
		space,
		uploadcmds.ListArguments{},
	)

	for _, r := range listOk.Results {
		fmt.Printf("%s\n", r.Root)
	}
}
