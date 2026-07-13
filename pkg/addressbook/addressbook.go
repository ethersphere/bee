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

// lastSeenUpdateInterval is the resolution at which UpdateLastSeen persists.
// Hive reports the same peer on every gossip round, while pruning operates on
// a scale of weeks, so a write is skipped when the stored value is already
// this fresh.
const lastSeenUpdateInterval = 24 * time.Hour

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
	// UpdateLastSeen marks the overlay as seen at the current time.
	UpdateLastSeen(overlay swarm.Address) error
	// Overlays returns a list of all overlay addresses saved in addressbook.
	Overlays() ([]swarm.Address, error)
	// IterateOverlays exposes overlays in a form of an iterator.
	IterateOverlays(func(swarm.Address) (bool, error)) error
	// Addresses returns a list of all bzz.Address-es saved in addressbook.
	Addresses() ([]bzz.Address, error)
	// Prune removes all entries whose overlay has not been seen since the
	// given time.
	Prune(before time.Time) error
}

type GetPutter interface {
	Getter
	Putter
}

// GetPutUpdater is the addressbook surface needed by hive: it stores peers it
// learns about and refreshes the last-seen time of the ones it already knows.
type GetPutUpdater interface {
	GetPutter
	// UpdateLastSeen marks the overlay as seen at the current time.
	UpdateLastSeen(overlay swarm.Address) error
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

type store struct {
	store storage.StateStorer
	now   func() time.Time

	// mu serializes the read-modify-write in UpdateLastSeen against Put, so a
	// concurrent Put is not rolled back by a stale copy of the entry.
	mu sync.Mutex
}

// New creates new addressbook for state storer.
func New(storer storage.StateStorer) Interface {
	return &store{
		store: storer,
		now:   time.Now,
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

// UpdateLastSeen marks the overlay as seen at the current time. It is a no-op
// if the overlay is not present in the addressbook, or if the recorded time is
// younger than lastSeenUpdateInterval.
func (s *store) UpdateLastSeen(overlay swarm.Address) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := keyPrefix + overlay.String()
	v := &verifiedAddress{}
	if err := s.store.Get(key, v); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return err
	}

	now := s.now().Unix()
	if now-v.LastSeen < int64(lastSeenUpdateInterval.Seconds()) {
		return nil
	}

	v.LastSeen = now
	return s.store.Put(key, v)
}

func (s *store) Remove(overlay swarm.Address) error {
	return s.store.Delete(keyPrefix + overlay.String())
}

// Prune removes all entries whose overlay has not been seen since before.
// Entries without a recorded last-seen time (LastSeen == 0) are kept, leaving
// pruning to a later run once they have been observed.
func (s *store) Prune(before time.Time) error {
	cutoff := before.Unix()

	var stale []swarm.Address
	err := s.store.Iterate(keyPrefix, func(key, value []byte) (stop bool, err error) {
		entry := &verifiedAddress{}
		if err := json.Unmarshal(value, entry); err != nil {
			return true, err
		}
		if entry.LastSeen != 0 && entry.LastSeen < cutoff {
			addr, err := swarm.ParseHexAddress(strings.TrimPrefix(string(key), keyPrefix))
			if err != nil {
				return true, err
			}
			stale = append(stale, addr)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	for _, addr := range stale {
		if err := s.Remove(addr); err != nil {
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
