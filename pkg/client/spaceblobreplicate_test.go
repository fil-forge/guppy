package client_test

import "testing"

// TODO(forrest): the production Client.SpaceBlobReplicate is still implemented
// against go-ucanto (the legacy `invoke` path with go-libstoracha capabilities)
// and is disabled in the upload flow. Its test needs a go-ucanto server, which
// the ucantone ctestutil harness no longer provides. Re-enable and port this
// test once SpaceBlobReplicate is migrated to ucantone/libforge.
func TestSpaceBlobReplicate(t *testing.T) {
	t.Skip("SpaceBlobReplicate is not yet ported to ucantone (see TODO(forrest))")
}
