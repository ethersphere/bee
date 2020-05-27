// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook

import (
	"fmt"
	"strings"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const keyPrefix = "addressbook_entry_"

var _ Interface = (*store)(nil)

type Interface interface {
	GetPutter
	Overlays() ([]swarm.Address, error)
	Multiaddresses() ([]bzz.Address, error)
}

type GetPutter interface {
	Getter
	Putter
}

type Getter interface {
	Get(overlay swarm.Address) (addr bzz.Address, err error)
}

type Putter interface {
	Put(overlay swarm.Address, addr bzz.Address) (err error)
}

type store struct {
	store storage.StateStorer
}

func New(storer storage.StateStorer) Interface {
	return &store{
		store: storer,
	}
}

func (s *store) Get(overlay swarm.Address) (bzz.Address, error) {
	key := keyPrefix + overlay.String()
	v := bzz.Address{}
	err := s.store.Get(key, &v)
	if err != nil {
		return bzz.Address{}, err
	}
	return v, nil
}

func (s *store) Put(overlay swarm.Address, addr bzz.Address) (err error) {
	key := keyPrefix + overlay.String()
	return s.store.Put(key, &addr)
}

func (s *store) Overlays() (overlays []swarm.Address, err error) {
	err = s.store.Iterate(keyPrefix, func(key, _ []byte) (stop bool, err error) {
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
		overlays = append(overlays, addr)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return overlays, nil
}

func (s *store) Multiaddresses() (multis []bzz.Address, err error) {
	err = s.store.Iterate(keyPrefix, func(_, value []byte) (stop bool, err error) {
		entry := &bzz.Address{}
		err = entry.UnmarshalJSON(value)
		if err != nil {
			return true, err
		}

		multis = append(multis, *entry)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return multis, nil
}
