package client_test

import (
	"testing"

	blobcmds "github.com/fil-forge/libforge/commands/blob"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/command"
	"github.com/fil-forge/ucantone/ucan/delegation"
	"github.com/stretchr/testify/require"

	"github.com/fil-forge/guppy/pkg/client"
	"github.com/fil-forge/guppy/pkg/presets"
	"github.com/fil-forge/guppy/pkg/tokenstore"
)

// newSpacesClient returns a client with an in-memory token store the test can
// populate via AddProofs.
func newSpacesClient(t *testing.T) *client.Client {
	t.Helper()
	return testutil.Must(client.New(
		testutil.RandomMultikeyIssuer(t),
		presets.DefaultNetwork.UploadID,
		presets.DefaultNetwork.UploadURL,
		client.WithTokenStore(tokenstore.NewMemStore()),
	))(t)
}

// spaceGrant builds a delegation from the space to the client's agent (the way a
// space grants access), optionally with options such as a name or expiry.
func spaceGrant(t *testing.T, c *client.Client, space ucan.Issuer, opts ...delegation.Option) ucan.Delegation {
	t.Helper()
	return testutil.Must(delegation.Delegate(space, c.Issuer().DID(), space.DID(), blobcmds.Add.Command, opts...))(t)
}

func TestSpaces(t *testing.T) {
	t.Run("returns empty when no space proofs exist", func(t *testing.T) {
		spaces, err := newSpacesClient(t).Spaces(t.Context())
		require.NoError(t, err)
		require.Empty(t, spaces)
	})

	t.Run("returns the space subject of a grant", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(), spaceGrant(t, c, space)))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Len(t, spaces, 1)
		require.Equal(t, space.DID(), spaces[0].DID())
	})

	t.Run("returns multiple unique spaces", func(t *testing.T) {
		c := newSpacesClient(t)
		space1 := testutil.RandomIssuer(t)
		space2 := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(), spaceGrant(t, c, space1), spaceGrant(t, c, space2)))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Len(t, spaces, 2)
		require.ElementsMatch(t,
			[]string{space1.DID().String(), space2.DID().String()},
			[]string{spaces[0].DID().String(), spaces[1].DID().String()},
		)
	})

	t.Run("deduplicates a space from multiple grants", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		// Two distinct grants (different issuers => different CIDs) for one space.
		require.NoError(t, c.AddProofs(t.Context(),
			spaceGrant(t, c, space),
			testutil.Must(delegation.Delegate(testutil.RandomIssuer(t), c.Issuer().DID(), space.DID(), blobcmds.Add.Command))(t),
		))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Len(t, spaces, 1)
		require.Equal(t, space.DID(), spaces[0].DID())
		require.Len(t, spaces[0].AccessProofs(), 2)
	})

	t.Run("skips ucan/attest delegations", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		attestSpace := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(),
			spaceGrant(t, c, space),
			testutil.Must(delegation.Delegate(attestSpace, c.Issuer().DID(), attestSpace.DID(), command.MustParse("/ucan/attest")))(t),
		))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Len(t, spaces, 1)
		require.Equal(t, space.DID(), spaces[0].DID())
	})

	t.Run("excludes expired and not-yet-valid delegations", func(t *testing.T) {
		c := newSpacesClient(t)
		expiredSpace := testutil.RandomIssuer(t)
		futureSpace := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(),
			spaceGrant(t, c, expiredSpace, delegation.WithExpiration(ucan.Now()-100)),
			spaceGrant(t, c, futureSpace, delegation.WithNotBefore(ucan.Now()+100)),
		))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Empty(t, spaces)
	})
}

func TestSpace(t *testing.T) {
	t.Run("Names returns names from grant metadata", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(),
			spaceGrant(t, c, space, delegation.WithMetadata(client.SpaceNameMetadata("my cool space"))),
		))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Len(t, spaces, 1)
		require.Equal(t, []string{"my cool space"}, spaces[0].Names())
	})

	t.Run("Names is empty when no name metadata is present", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(), spaceGrant(t, c, space)))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Len(t, spaces, 1)
		require.Empty(t, spaces[0].Names())
	})

	t.Run("AccessProofs returns the grants for a space", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		grant := spaceGrant(t, c, space)
		require.NoError(t, c.AddProofs(t.Context(), grant))

		spaces, err := c.Spaces(t.Context())
		require.NoError(t, err)
		require.Len(t, spaces, 1)
		proofs := spaces[0].AccessProofs()
		require.Len(t, proofs, 1)
		require.Equal(t, grant.Link(), proofs[0].Link())
	})
}

func TestSpaceNamed(t *testing.T) {
	t.Run("returns SpaceNotFoundError when no space has the name", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(), spaceGrant(t, c, space, delegation.WithMetadata(client.SpaceNameMetadata("some name")))))

		_, err := c.SpaceNamed(t.Context(), "different name")
		var notFound client.SpaceNotFoundError
		require.ErrorAs(t, err, &notFound)
		require.Equal(t, "different name", notFound.Name)
	})

	t.Run("returns the space with the matching name", func(t *testing.T) {
		c := newSpacesClient(t)
		space := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(), spaceGrant(t, c, space, delegation.WithMetadata(client.SpaceNameMetadata("my space")))))

		result, err := c.SpaceNamed(t.Context(), "my space")
		require.NoError(t, err)
		require.Equal(t, space.DID(), result.DID())
	})

	t.Run("returns MultipleSpacesFoundError when the name is ambiguous", func(t *testing.T) {
		c := newSpacesClient(t)
		space1 := testutil.RandomIssuer(t)
		space2 := testutil.RandomIssuer(t)
		require.NoError(t, c.AddProofs(t.Context(),
			spaceGrant(t, c, space1, delegation.WithMetadata(client.SpaceNameMetadata("shared"))),
			spaceGrant(t, c, space2, delegation.WithMetadata(client.SpaceNameMetadata("shared"))),
		))

		_, err := c.SpaceNamed(t.Context(), "shared")
		var multiple client.MultipleSpacesFoundError
		require.ErrorAs(t, err, &multiple)
		require.Equal(t, "shared", multiple.Name)
	})
}
