package tokenstore

import (
	ucanlib "github.com/fil-forge/libforge/ucan"
	"github.com/fil-forge/ucantone/ucan"
)

type Store interface {
	ucanlib.ProofStore
	AddInvocations(invocations ...ucan.Invocation) error
	AddDelegations(delegations ...ucan.Delegation) error
	AddReceipts(receipts ...ucan.Receipt) error
	Reset() error
}
