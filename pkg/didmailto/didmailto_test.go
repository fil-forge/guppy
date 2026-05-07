package didmailto_test

import (
	"testing"

	"github.com/fil-forge/guppy/pkg/didmailto"
	"github.com/stretchr/testify/require"
)

func TestFromEmail(t *testing.T) {
	t.Run("with valid email", func(t *testing.T) {
		did, err := didmailto.FromEmail("alice@example.com")
		require.NoError(t, err)
		require.Equal(t, "did:mailto:example.com:alice", did.String())
	})

	t.Run("with invalid email", func(t *testing.T) {
		_, err := didmailto.FromEmail("invalid-email")
		require.ErrorContains(t, err, "invalid email address")
	})

	t.Run("with escape-worthy characters", func(t *testing.T) {
		did, err := didmailto.FromEmail("alice+test@example.com")
		require.NoError(t, err)
		require.Equal(t, "did:mailto:example.com:alice%2Btest", did.String())
	})
}
