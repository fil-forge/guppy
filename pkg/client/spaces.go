package client

import (
	"bytes"
	"context"
	"fmt"
	"slices"

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
func (c *Client) Spaces() ([]Space, error) {
	dels, err := c.tokenStore.Delegations(context.Background())
	if err != nil {
		return nil, fmt.Errorf("listing delegations: %w", err)
	}

	agent := c.signer.DID()
	now := ucan.Now()

	order := make([]did.DID, 0)
	proofs := map[did.DID][]ucan.Delegation{}
	for _, d := range dels {
		if d.Audience() != agent {
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
		if !sub.Defined() || sub == agent {
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
		if _, ok := proofs[sub]; !ok {
			order = append(order, sub)
		}
		proofs[sub] = append(proofs[sub], d)
	}

	spaces := make([]Space, 0, len(order))
	for _, sub := range order {
		spaces = append(spaces, Space{
			did:          sub,
			accessProofs: deduplicateDelegations(proofs[sub]),
		})
	}
	return spaces, nil
}

// SpacesNamed returns all spaces with the given name.
func (c *Client) SpacesNamed(name string) ([]Space, error) {
	spaces, err := c.Spaces()
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
func (c *Client) SpaceNamed(name string) (Space, error) {
	spaces, err := c.SpacesNamed(name)
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

// deduplicateDelegations removes duplicate delegations by CID.
func deduplicateDelegations(dels []ucan.Delegation) []ucan.Delegation {
	seen := make(map[cid.Cid]struct{})
	result := make([]ucan.Delegation, 0, len(dels))
	for _, d := range dels {
		if _, exists := seen[d.Link()]; exists {
			continue
		}
		seen[d.Link()] = struct{}{}
		result = append(result, d)
	}
	return slices.Clip(result)
}
