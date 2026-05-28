package client

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fil-forge/guppy/pkg/tokenstore"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ipld/datamodel"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/ipfs/go-cid"
)

// SpaceNameMetaKey is the delegation-metadata key under which a space's name is
// recorded when access to a space is granted (see the `space generate` command).
const SpaceNameMetaKey = "name"

// attestCommand identifies ucan/attest session proofs, which attest to other
// proofs rather than granting access to a space.
const attestCommand = "/ucan/attest"

// SpaceNotFoundError is returned when no space is found with a given name.
type SpaceNotFoundError struct {
	Name string
}

func (e SpaceNotFoundError) Error() string {
	return fmt.Sprintf("no space found with name %q", e.Name)
}

// MultipleSpacesFoundError is returned when multiple spaces are found with the same name.
type MultipleSpacesFoundError struct {
	Name   string
	Spaces []Space
}

func (e MultipleSpacesFoundError) Error() string {
	return fmt.Sprintf("multiple spaces found with name %q", e.Name)
}

// Space represents a space we can act as, along with the proofs that grant access.
type Space struct {
	did          did.DID
	accessProofs []ucan.Delegation
}

// DID returns the DID of this space.
func (s Space) DID() did.DID {
	return s.did
}

// AccessProofs returns the delegations that grant access to this space.
func (s Space) AccessProofs() []ucan.Delegation {
	return s.accessProofs
}

// Names returns the names recorded in the metadata of this space's access
// proofs. Typically a space has a single name, but multiple are possible when
// several delegations (each with their own name) grant access to it.
func (s Space) Names() []string {
	var names []string
	seen := map[string]struct{}{}
	for _, proof := range s.accessProofs {
		mb := proof.MetadataBytes()
		if len(mb) == 0 {
			continue
		}
		var meta datamodel.Map
		if err := meta.UnmarshalCBOR(bytes.NewReader(mb)); err != nil {
			continue
		}
		v, ok := meta[SpaceNameMetaKey]
		if !ok {
			continue
		}
		name, ok := v.(string)
		if !ok || name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// SpaceNameMetadata returns delegation metadata that records a space's name. It
// is attached when granting access to a space so that [Space.Names] can later
// recover the name.
func SpaceNameMetadata(name string) datamodel.Map {
	return datamodel.Map{SpaceNameMetaKey: name}
}

// Spaces returns all spaces we can act as, derived from the delegations held by
// the token store: each distinct subject of a (valid, non-attestation)
// delegation addressed to this agent is a space.
func (c *Client) Spaces(ctx context.Context) ([]Space, error) {
	// Get direct delegations to the agent
	dlgs, err := delegationsForAudience(ctx, c.tokenStore, c.signer.DID())
	if err != nil {
		return nil, fmt.Errorf("getting delegations for agent: %w", err)
	}

	accs, err := c.Accounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting accounts: %w", err)
	}

	// Get delegations to the agent's accounts
	for _, acc := range accs {
		accDlgs, err := delegationsForAudience(ctx, c.tokenStore, acc)
		if err != nil {
			return nil, fmt.Errorf("getting delegations for account %s: %w", acc, err)
		}
		dlgs = append(dlgs, accDlgs...)
	}

	// Group delegations by subject (space DID)
	proofs := map[did.DID]map[cid.Cid]ucan.Delegation{}
	for _, d := range dlgs {
		sub := d.Subject()
		dlgs, ok := proofs[sub]
		if !ok {
			dlgs = map[cid.Cid]ucan.Delegation{}
			proofs[sub] = dlgs
		}
		dlgs[d.Link()] = d
	}

	spaces := make([]Space, 0, len(proofs))
	for space, dlgs := range proofs {
		spaces = append(spaces, Space{
			did:          space,
			accessProofs: slices.Collect(maps.Values(dlgs)),
		})
	}
	slices.SortFunc(spaces, func(a, b Space) int {
		return strings.Compare(a.DID().String(), b.DID().String())
	})
	return spaces, nil
}

func delegationsForAudience(ctx context.Context, store tokenstore.Store, aud did.DID) ([]ucan.Delegation, error) {
	dels, err := store.Delegations(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing delegations: %w", err)
	}

	now := ucan.Now()
	var proofs []ucan.Delegation
	for _, d := range dels {
		if d.Audience() != aud {
			continue
		}
		if d.Command().String() == attestCommand {
			continue
		}
		if exp := d.Expiration(); exp != nil && *exp < now {
			continue
		}
		if nb := d.NotBefore(); nb != nil && *nb > now {
			continue
		}
		sub := d.Subject()
		if !sub.Defined() || sub == aud {
			continue
		}
		// Skip account-root delegations. Sprue's access/confirm issues
		// a root delegation from the account to the agent with
		// subject == account.DID() (a did:mailto), required by the UCAN
		// spec — see ucantone/validator/validator.go's "root delegation
		// subject is null" check. That delegation represents access to
		// the account itself, not to a space, so it shouldn't be listed
		// here. Spaces always use did:key subjects.
		if sub.Method() == "mailto" {
			continue
		}
		proofs = append(proofs, d)
	}

	return proofs, nil
}

// SpacesNamed returns all spaces with the given name.
func (c *Client) SpacesNamed(ctx context.Context, name string) ([]Space, error) {
	spaces, err := c.Spaces(ctx)
	if err != nil {
		return nil, err
	}

	var result []Space
	for _, space := range spaces {
		if slices.Contains(space.Names(), name) {
			result = append(result, space)
		}
	}
	return result, nil
}

// SpaceNamed returns the single space with the given name. Returns
// [SpaceNotFoundError] if no space is found. Returns [MultipleSpacesFoundError]
// if multiple spaces have the same name.
func (c *Client) SpaceNamed(ctx context.Context, name string) (Space, error) {
	spaces, err := c.SpacesNamed(ctx, name)
	if err != nil {
		return Space{}, err
	}

	if len(spaces) == 0 {
		return Space{}, SpaceNotFoundError{Name: name}
	}
	if len(spaces) > 1 {
		return Space{}, MultipleSpacesFoundError{Name: name, Spaces: spaces}
	}
	return spaces[0], nil
}
