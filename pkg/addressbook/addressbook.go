// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const keyPrefix = "addressbook_entry_"

var _ Interface = (*store)(nil)

var ErrNotFound = errors.New("addressbook: not found")

// verifiedAddress pairs a bzz.Address with a flag indicating whether the peer
// has been verified, and the last time the overlay was seen.
type verifiedAddress struct {
	Address  *bzz.Address `json:"address"`
	Verified bool         `json:"verified"`
	// LastSeen is the Unix timestamp (seconds) of the last time the overlay
	// was seen over hive or in kademlia. Used to prune stale entries.
	LastSeen int64 `json:"last_seen"`
}

// Interface is the AddressBook interface.
type Interface interface {
	GetPutter
	Remover
	Seener
	// Overlays returns a list of all overlay addresses saved in addressbook.
	Overlays() ([]swarm.Address, error)
	// IterateOverlays exposes overlays in a form of an iterator.
	IterateOverlays(func(swarm.Address) (bool, error)) error
	// Addresses returns a list of all bzz.Address-es saved in addressbook.
	Addresses() ([]bzz.Address, error)
}

type GetPutter interface {
	Getter
	Putter
}

// GetPutSeener is the addressbook surface needed by hive: it stores the peers
// it learns about and marks the ones it already knows as seen.
type GetPutSeener interface {
	GetPutter
	Seener
}

type Getter interface {
	// Get returns the saved bzz.Address for the requested overlay together
	// with its verification flag.
	Get(overlay swarm.Address) (addr *bzz.Address, verified bool, err error)
}

type Putter interface {
	// Put saves relation between peer overlay address and bzz.Address address with verification flag.
	Put(overlay swarm.Address, addr bzz.Address, verified bool) (err error)
}

type Remover interface {
	// Remove removes overlay address.
	Remove(overlay swarm.Address) error
}

type Seener interface {
	// Seen marks the overlays as seen at the current time.
	Seen(overlays ...swarm.Address) error
}

type store struct {
	store storage.StateStorer
	now   func() time.Time

	// mu serializes the read-modify-write in Seen against Put, so that a
	// concurrent Put is not rolled back by a stale copy of the entry.
	mu sync.Mutex
}

// New creates new addressbook for state storer.
func New(storer storage.StateStorer) Interface {
	return newStore(storer, time.Now)
}

func newStore(storer storage.StateStorer, now func() time.Time) *store {
	return &store{
		store: storer,
		now:   now,
	}
}

func (s *store) Get(overlay swarm.Address) (*bzz.Address, bool, error) {
	key := keyPrefix + overlay.String()
	v := &verifiedAddress{}
	err := s.store.Get(key, v)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	return v.Address, v.Verified, nil
}

func (s *store) Put(overlay swarm.Address, addr bzz.Address, verified bool) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := keyPrefix + overlay.String()
	return s.store.Put(key, &verifiedAddress{
		Address:  &addr,
		Verified: verified,
		LastSeen: s.now().Unix(),
	})
}

// Seen marks the overlays as seen at the current time. An overlay that is not
// present in the addressbook is skipped.
func (s *store) Seen(overlays ...swarm.Address) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().Unix()

	for _, overlay := range overlays {
		key := keyPrefix + overlay.String()

		v := &verifiedAddress{}
		if err := s.store.Get(key, v); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return err
		}

		v.LastSeen = now
		if err := s.store.Put(key, v); err != nil {
			return err
		}
	}

	return nil
}

func (s *store) Remove(overlay swarm.Address) error {
	return s.store.Delete(keyPrefix + overlay.String())
}

// Prune removes all entries from storer whose overlay has not been seen since
// before. Entries without a recorded last-seen time (LastSeen == 0) are kept,
// leaving them to a later run once they have been observed. It is a one-shot
// maintenance step meant to run at startup, before the address book is shared
// with its writers, so it takes no lock.
func Prune(storer storage.StateStorer, before time.Time) error {
	cutoff := before.Unix()

	var stale []string
	err := storer.Iterate(keyPrefix, func(key, value []byte) (stop bool, err error) {
		entry := &verifiedAddress{}
		if err := json.Unmarshal(value, entry); err != nil {
			return true, err
		}
		if entry.LastSeen != 0 && entry.LastSeen < cutoff {
			stale = append(stale, string(key))
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	for _, key := range stale {
		if err := storer.Delete(key); err != nil {
			return err
		}
	}

	return nil
}

func (s *store) IterateOverlays(cb func(swarm.Address) (bool, error)) error {
	return s.store.Iterate(keyPrefix, func(key, _ []byte) (stop bool, err error) {
		k := string(key)
		if !strings.HasPrefix(k, keyPrefix) {
			return true, nil
		}
		split := strings.SplitAfter(k, keyPrefix)
		if len(split) != 2 {
			return true, fmt.Errorf("invalid overlay key: %s", k)
		}
		addr, err := swarm.ParseHexAddress(split[1])
		if err != nil {
			return true, err
		}
		stop, err = cb(addr)
		if err != nil {
			return true, err
		}
		if stop {
			return true, nil
		}
		return false, nil
	})
}

func (s *store) Overlays() (overlays []swarm.Address, err error) {
	err = s.IterateOverlays(func(addr swarm.Address) (stop bool, err error) {
		overlays = append(overlays, addr)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return overlays, nil
}

func (s *store) Addresses() (addresses []bzz.Address, err error) {
	err = s.store.Iterate(keyPrefix, func(_, value []byte) (stop bool, err error) {
		entry := &verifiedAddress{}
		if err := json.Unmarshal(value, entry); err != nil {
			return true, err
		}
		if entry.Address == nil {
			return false, nil
		}
		addresses = append(addresses, *entry.Address)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return addresses, nil
}
