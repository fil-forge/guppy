package tokenstore

import (
	"context"
	"iter"

	ucanlib "github.com/fil-forge/libforge/ucan"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan"
)

type Store interface {
	ucanlib.ProofStore
	// AddInvocations adds the given invocations to the store.
	AddInvocations(ctx context.Context, invocations ...ucan.Invocation) error
	// AddDelegations adds the given delegations to the store.
	AddDelegations(ctx context.Context, delegations ...ucan.Delegation) error
	// AddReceipts adds the given receipts to the store.
	AddReceipts(ctx context.Context, receipts ...ucan.Receipt) error
	// ListDelegations returns a sequence of delegations matching the given criteria.
	ListDelegations(ctx context.Context, aud did.DID, cmd ucan.Command, sub did.DID) iter.Seq2[ucan.Delegation, error]
	// Delegations returns all delegations held by the store.
	Delegations(ctx context.Context) ([]ucan.Delegation, error)
	// Reset clears all data in the store.
	Reset(ctx context.Context) error
}
