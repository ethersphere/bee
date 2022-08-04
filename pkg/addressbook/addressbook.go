// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook

import (
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const keyPrefix = "addressbook_entry_"

var _ Store = (*store)(nil)

var ErrNotFound = errors.New("addressbook: not found")

// Store is state storage wrapper which maps swarm.Address to bzz.Address types.
type Store interface {
	GetPutter
	// Remove removes overlay address.
	Remove(overlay swarm.Address) error
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

type Getter interface {
	// Get returns pointer to saved bzz.Address for requested overlay address.
	Get(overlay swarm.Address) (addr *bzz.Address, err error)
}

type Putter interface {
	// Put saves relation between peer overlay address and bzz.Address address.
	Put(overlay swarm.Address, addr bzz.Address) (err error)
}

type store struct {
	store storage.EntityStore
}

// New creates new addressbook for state storer.
func New(storer storage.StateStorer) Store {
	return &store{
		store: storage.NewEntityStore(storer, keyFromEntityFunc, entityFromKeyFunc, valueUnmarshalFunc),
	}
}

func (s *store) Get(overlay swarm.Address) (*bzz.Address, error) {
	v := &bzz.Address{}

	if err := s.store.Get(overlay, v); err != nil {
		if err == storage.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return v, nil
}

func (s *store) Put(overlay swarm.Address, addr bzz.Address) (err error) {
	return s.store.Put(overlay, &addr)
}

func (s *store) Remove(overlay swarm.Address) error {
	return s.store.Delete(overlay)
}

func (s *store) IterateOverlays(cb func(swarm.Address) (bool, error)) error {
	return s.store.IterateKeys(keyPrefix, func(key interface{}) (stop bool, err error) {
		if overlay, ok := key.(swarm.Address); ok {
			stop, err = cb(overlay)
			if err != nil {
				return true, err
			}
			if stop {
				return true, nil
			}
		}

		return false, nil
	})
}

func (s *store) Overlays() (overlays []swarm.Address, err error) {
	err = s.store.IterateKeys(keyPrefix, func(key interface{}) (stop bool, err error) {
		if overlay, ok := key.(swarm.Address); ok {
			overlays = append(overlays, overlay)
		}

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return overlays, nil
}

func (s *store) Addresses() (addresses []bzz.Address, err error) {
	err = s.store.IterateValues(keyPrefix, func(value interface{}) (stop bool, err error) {
		if addr, ok := value.(bzz.Address); ok {
			addresses = append(addresses, addr)
		}

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return addresses, nil
}

func keyFromEntityFunc(key interface{}) string {
	overlay := key.(swarm.Address)
	return keyPrefix + overlay.String()
}

func entityFromKeyFunc(key string) (interface{}, error) {
	addr, err := swarm.ParseHexAddress(key[len(keyPrefix):])
	if err != nil {
		return nil, fmt.Errorf("invalid overlay key: %s, err: %w", key, err)
	}

	return addr, nil
}

func valueUnmarshalFunc(v []byte) (interface{}, error) {
	value := &bzz.Address{}

	if err := value.UnmarshalJSON(v); err != nil {
		return nil, err
	}

	return *value, nil
}
