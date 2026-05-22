package preparation_test

import "testing"

// TODO(forrest): port the end-to-end preparation test to ucantone.
//
// The original test (preserved in git history immediately before this commit)
// stands up a full fake Storacha service via ctestutil using the go-ucanto
// server harness (server.WithServiceMethod / server.Provide handlers for
// space/index/add, space/blob/replicate, filecoin/offer, upload/add) and drives
// the whole scan -> DAG -> shard -> upload pipeline against it.
//
// Porting requires rebuilding that fake service as ucantone server routes
// (ctestutil.WithServerRoutes + server.NewRoute with libforge command bindings,
// as in pkg/client/testutil/blobadd.go's BlobAddHandler), and reducing the
// replicate/offer assertions like pkg/preparation/storacha/storacha_test.go
// (those steps are commented out in the production storacha API).
//
// The production pipeline is already ported; this is test-only work. Follow the
// patterns in pkg/client/*_test.go (server routes) and storacha_test.go.
func TestPreparation(t *testing.T) {
	t.Skip("end-to-end preparation test needs a ucantone fake-service port (see TODO(forrest))")
}
