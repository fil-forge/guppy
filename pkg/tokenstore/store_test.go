package tokenstore_test

import (
	"slices"
	"testing"

	"github.com/fil-forge/guppy/pkg/tokenstore"
	"github.com/fil-forge/ucantone/principal/ed25519"
	"github.com/fil-forge/ucantone/testutil"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/command"
	"github.com/fil-forge/ucantone/ucan/delegation"
	"github.com/fil-forge/ucantone/ucan/invocation"
	"github.com/fil-forge/ucantone/ucan/receipt"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

func delegationLinks(dels []ucan.Delegation) []cid.Cid {
	links := make([]cid.Cid, len(dels))
	for i, d := range dels {
		links[i] = d.Link()
	}
	return links
}

func collectDelegations(t *testing.T, seq func(yield func(ucan.Delegation, error) bool)) []ucan.Delegation {
	t.Helper()
	var out []ucan.Delegation
	for d, err := range seq {
		require.NoError(t, err)
		out = append(out, d)
	}
	return out
}

func TestStore(t *testing.T) {
	stores := map[string]func(t *testing.T) tokenstore.Store{
		"MemStore": func(t *testing.T) tokenstore.Store {
			return tokenstore.NewMemStore()
		},
		"FsStore": func(t *testing.T) tokenstore.Store {
			s, err := tokenstore.NewFsStore(t.TempDir())
			require.NoError(t, err)
			return s
		},
	}

	for name, newStore := range stores {
		t.Run(name, func(t *testing.T) {
			t.Run("Delegations", func(t *testing.T) { testDelegations(t, newStore) })
			t.Run("Invocations", func(t *testing.T) { testInvocations(t, newStore) })
			t.Run("Receipts", func(t *testing.T) { testReceipts(t, newStore) })
			t.Run("ListDelegations", func(t *testing.T) { testListDelegations(t, newStore) })
			t.Run("Reset", func(t *testing.T) { testReset(t, newStore) })
		})
	}
}

func testDelegations(t *testing.T, newStore func(t *testing.T) tokenstore.Store) {
	cmd1, err := command.Parse("/test/action")
	require.NoError(t, err)

	t.Run("empty initially", func(t *testing.T) {
		s := newStore(t)
		signer := testutil.RandomSigner(t)
		result := collectDelegations(t, s.ListDelegations(t.Context(), signer.DID(), cmd1, signer.DID()))
		require.Empty(t, result)
	})

	t.Run("add one delegation", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		aud := testutil.RandomSigner(t)

		dlg, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)

		require.NoError(t, s.AddDelegations(t.Context(), dlg))

		result := collectDelegations(t, s.ListDelegations(t.Context(), aud.DID(), cmd1, issuer.DID()))
		require.Len(t, result, 1)
		require.Equal(t, dlg.Link(), result[0].Link())
	})

	t.Run("add multiple delegations", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		aud := testutil.RandomSigner(t)

		dlg1, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)
		dlg2, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)

		require.NoError(t, s.AddDelegations(t.Context(), dlg1, dlg2))

		result := collectDelegations(t, s.ListDelegations(t.Context(), aud.DID(), cmd1, issuer.DID()))
		require.Len(t, result, 2)
		require.ElementsMatch(t, delegationLinks([]ucan.Delegation{dlg1, dlg2}), delegationLinks(result))
	})
}

func testInvocations(t *testing.T, newStore func(t *testing.T) tokenstore.Store) {
	cmd1, err := command.Parse("/test/action")
	require.NoError(t, err)

	t.Run("add invocation does not error", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		sub := testutil.RandomSigner(t)

		inv, err := invocation.Invoke(issuer, sub.DID(), cmd1, testutil.RandomArgs(t))
		require.NoError(t, err)

		require.NoError(t, s.AddInvocations(t.Context(), inv))
	})

	t.Run("add multiple invocations does not error", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		sub := testutil.RandomSigner(t)

		inv1, err := invocation.Invoke(issuer, sub.DID(), cmd1, testutil.RandomArgs(t))
		require.NoError(t, err)
		inv2, err := invocation.Invoke(issuer, sub.DID(), cmd1, testutil.RandomArgs(t))
		require.NoError(t, err)

		require.NoError(t, s.AddInvocations(t.Context(), inv1, inv2))
	})
}

func testReceipts(t *testing.T, newStore func(t *testing.T) tokenstore.Store) {
	t.Run("add receipt does not error", func(t *testing.T) {
		s := newStore(t)
		executor := testutil.RandomSigner(t)
		ran := testutil.RandomCID(t)

		rcpt, err := receipt.IssueOK(executor, ran, testutil.RandomArgs(t))
		require.NoError(t, err)

		require.NoError(t, s.AddReceipts(t.Context(), rcpt))
	})

	t.Run("add multiple receipts does not error", func(t *testing.T) {
		s := newStore(t)
		executor := testutil.RandomSigner(t)

		rcpt1, err := receipt.IssueOK(executor, testutil.RandomCID(t), testutil.RandomArgs(t))
		require.NoError(t, err)
		rcpt2, err := receipt.IssueOK(executor, testutil.RandomCID(t), testutil.RandomArgs(t))
		require.NoError(t, err)

		require.NoError(t, s.AddReceipts(t.Context(), rcpt1, rcpt2))
	})
}

func testListDelegations(t *testing.T, newStore func(t *testing.T) tokenstore.Store) {
	cmd1, err := command.Parse("/test/action")
	require.NoError(t, err)
	cmd2, err := command.Parse("/test/other")
	require.NoError(t, err)

	t.Run("filters by audience", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		aud1 := testutil.RandomSigner(t)
		aud2 := testutil.RandomSigner(t)

		dlg1, err := delegation.Delegate(issuer, aud1.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)
		dlg2, err := delegation.Delegate(issuer, aud2.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)

		require.NoError(t, s.AddDelegations(t.Context(), dlg1, dlg2))

		result := collectDelegations(t, s.ListDelegations(t.Context(), aud1.DID(), cmd1, issuer.DID()))
		require.Len(t, result, 1)
		require.Equal(t, dlg1.Link(), result[0].Link())
	})

	t.Run("filters by command", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		aud := testutil.RandomSigner(t)

		dlg1, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)
		dlg2, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd2)
		require.NoError(t, err)

		require.NoError(t, s.AddDelegations(t.Context(), dlg1, dlg2))

		result := collectDelegations(t, s.ListDelegations(t.Context(), aud.DID(), cmd1, issuer.DID()))
		require.Len(t, result, 1)
		require.Equal(t, dlg1.Link(), result[0].Link())
	})

	t.Run("filters by subject", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		aud := testutil.RandomSigner(t)
		sub2 := testutil.RandomSigner(t)

		dlg1, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)
		dlg2, err := delegation.Delegate(issuer, aud.DID(), sub2.DID(), cmd1)
		require.NoError(t, err)

		require.NoError(t, s.AddDelegations(t.Context(), dlg1, dlg2))

		result := collectDelegations(t, s.ListDelegations(t.Context(), aud.DID(), cmd1, issuer.DID()))
		require.Len(t, result, 1)
		require.Equal(t, dlg1.Link(), result[0].Link())
	})

	t.Run("returns empty when nothing matches", func(t *testing.T) {
		s := newStore(t)
		result := collectDelegations(t, s.ListDelegations(t.Context(), testutil.RandomDID(t), cmd1, testutil.RandomDID(t)))
		require.Empty(t, result)
	})
}

func testReset(t *testing.T, newStore func(t *testing.T) tokenstore.Store) {
	cmd1, err := command.Parse("/test/action")
	require.NoError(t, err)

	t.Run("clears all delegations", func(t *testing.T) {
		s := newStore(t)
		issuer := testutil.RandomSigner(t)
		aud := testutil.RandomSigner(t)

		dlg, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd1)
		require.NoError(t, err)

		require.NoError(t, s.AddDelegations(t.Context(), dlg))
		result := collectDelegations(t, s.ListDelegations(t.Context(), aud.DID(), cmd1, issuer.DID()))
		require.Len(t, result, 1)

		require.NoError(t, s.Reset(t.Context()))

		result = collectDelegations(t, s.ListDelegations(t.Context(), aud.DID(), cmd1, issuer.DID()))
		require.Empty(t, result)
	})
}

func TestFsStorePersistence(t *testing.T) {
	cmd1, err := command.Parse("/test/action")
	require.NoError(t, err)

	dir := t.TempDir()

	issuer, err := ed25519.Generate()
	require.NoError(t, err)
	aud, err := ed25519.Generate()
	require.NoError(t, err)

	dlg, err := delegation.Delegate(issuer, aud.DID(), issuer.DID(), cmd1)
	require.NoError(t, err)

	s1, err := tokenstore.NewFsStore(dir)
	require.NoError(t, err)
	require.NoError(t, s1.AddDelegations(t.Context(), dlg))

	s2, err := tokenstore.NewFsStore(dir)
	require.NoError(t, err)

	result := collectDelegations(t, s2.ListDelegations(t.Context(), aud.DID(), cmd1, issuer.DID()))
	require.Len(t, result, 1)
	require.True(t, slices.ContainsFunc(result, func(d ucan.Delegation) bool {
		return d.Link() == dlg.Link()
	}), "persisted delegation should be present after reload")
}
