package client_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	accesscmds "github.com/fil-forge/libforge/commands/access"
	uploadcmds "github.com/fil-forge/libforge/commands/upload"
	"github.com/fil-forge/libforge/testutil"
	"github.com/fil-forge/ucantone/binding"
	"github.com/fil-forge/ucantone/ipld/datamodel"
	"github.com/fil-forge/ucantone/server"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/container"
	"github.com/fil-forge/ucantone/ucan/delegation"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	ctestutil "github.com/fil-forge/guppy/pkg/client/testutil"
)

type claimResponse struct {
	dels []ucan.Delegation
	err  error
}

func TestPollClaim(t *testing.T) {
	var responses []claimResponse

	c := testutil.Must(ctestutil.Client(t,
		ctestutil.WithServerRoutes(func(deps ctestutil.RouteDeps) server.Route {
			return accesscmds.Claim.Route(func(req *binding.Request[*accesscmds.ClaimArguments], res *binding.Response[*accesscmds.ClaimOK]) error {
				if len(responses) == 0 {
					return res.SetFailure(fmt.Errorf("no more responses available"))
				}
				var r claimResponse
				r, responses = responses[0], responses[1:]
				if r.err != nil {
					return res.SetFailure(r.err)
				}
				links := make([]cid.Cid, 0, len(r.dels))
				for _, d := range r.dels {
					links = append(links, d.Link())
				}
				if err := res.SetMetadata(container.New(container.WithDelegations(r.dels...))); err != nil {
					return err
				}
				return res.SetSuccess(&accesscmds.ClaimOK{Delegations: links})
			})
		}),
	))(t)

	requestLink := testutil.RandomCID(t)

	unrelatedDel := testutil.Must(uploadcmds.Add.Delegate(c.Issuer(), c.Issuer().DID(), c.Issuer().DID()))(t)
	relatedDel := testutil.Must(uploadcmds.Add.Delegate(
		c.Issuer(),
		c.Issuer().DID(),
		c.Issuer().DID(),
		delegation.WithMetadata(datamodel.Map{accesscmds.RequestMetaKey: requestLink}),
	))(t)

	requestOK := &accesscmds.RequestOK{Request: requestLink}

	t.Run("polls until it finds authorized delegations", func(t *testing.T) {
		responses = []claimResponse{
			{dels: nil},
			{dels: []ucan.Delegation{unrelatedDel}},
			{dels: []ucan.Delegation{unrelatedDel, relatedDel}},
		}

		tickChan := make(chan time.Time, 3)
		tickChan <- time.Now()
		tickChan <- time.Now()
		tickChan <- time.Now()

		resultChan := c.PollClaimWithTick(t.Context(), requestOK, tickChan)

		claimedDels, err := (<-resultChan).Unpack()
		require.NoError(t, err, "expected no error from PollClaim")
		require.Len(t, claimedDels.Delegations, 1, "expected exactly one delegation to be claimed")
		require.Equal(t, relatedDel.Link(), claimedDels.Delegations[0].Link(), "expected only the related delegation")

		_, ok := <-resultChan
		require.False(t, ok, "expected result channel to be closed after claim")
	})

	t.Run("reports an error during claim", func(t *testing.T) {
		responses = []claimResponse{
			{err: fmt.Errorf("Something went wrong!")},
		}

		tickChan := make(chan time.Time, 1)
		tickChan <- time.Now()

		resultChan := c.PollClaimWithTick(t.Context(), requestOK, tickChan)

		claimedDels, err := (<-resultChan).Unpack()
		require.Empty(t, claimedDels)
		require.ErrorContains(t, err, "Something went wrong!", "expected error from PollClaim")

		_, ok := <-resultChan
		require.False(t, ok, "expected result channel to be closed after error")
	})

	t.Run("respects the context's cancelation", func(t *testing.T) {
		responses = nil
		tickChan := make(chan time.Time) // never ticks
		ctx, cancel := context.WithCancel(t.Context())

		resultChan := c.PollClaimWithTick(ctx, requestOK, tickChan)
		cancel()

		claimedDels, err := (<-resultChan).Unpack()
		require.Empty(t, claimedDels)
		require.ErrorContains(t, err, "context canceled")

		_, ok := <-resultChan
		require.False(t, ok, "expected result channel to be closed after cancelation")
	})
}
