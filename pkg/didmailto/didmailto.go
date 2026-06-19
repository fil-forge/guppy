package didmailto

import (
	"strings"

	"github.com/fil-forge/libforge/attestation/didmailto"
	"github.com/fil-forge/ucantone/did"
)

// FromInput converts either a `did:mailto:` DID or an email address to a
// `did:mailto:` DID.
func FromInput(input string) (did.DID, error) {
	if strings.HasPrefix(input, "did:mailto:") {
		return did.Parse(input)
	}
	return didmailto.New(input)
}
