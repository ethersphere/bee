// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

const keyPrefix = "addressbook_entry_"

var _ GetPutter = (*store)(nil)

type GetPutter interface {
	Getter
	Putter
}

type Getter interface {
	Get(overlay swarm.Address) (addr ma.Multiaddr, err error)
}

type Putter interface {
	Put(overlay swarm.Address, addr ma.Multiaddr) (err error)
}

type store struct {
	store storage.StateStorer
}

func New(storer storage.StateStorer) GetPutter {
	return &store{
		store: storer,
	}
}

func (s *store) Get(overlay swarm.Address) (ma.Multiaddr, error) {
	key := keyPrefix + overlay.String()
	v := PeerEntry{}

	err := s.store.Get(key, &v)
	if err != nil {
		return nil, err
	}
	return v.Multiaddr, nil
}

func (s *store) Put(overlay swarm.Address, addr ma.Multiaddr) (err error) {
	key := keyPrefix + overlay.String()
	pe := &PeerEntry{Overlay: overlay, Multiaddr: addr}

	return s.store.Put(key, pe)
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

func (s *store) Multiaddresses() (multis []ma.Multiaddr, err error) {
	err = s.store.Iterate(keyPrefix, func(_, value []byte) (stop bool, err error) {
		entry := &PeerEntry{}
		err = entry.UnmarshalJSON(value)
		if err != nil {
			return true, err
		}

		multis = append(multis, entry.Multiaddr)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return multis, nil
}

type PeerEntry struct {
	Overlay   swarm.Address
	Multiaddr ma.Multiaddr
}

func (p *PeerEntry) MarshalJSON() ([]byte, error) {
	v := struct {
		Overlay   string
		Multiaddr string
	}{
		Overlay:   p.Overlay.String(),
		Multiaddr: p.Multiaddr.String(),
	}
	return json.Marshal(&v)
}

func (p *PeerEntry) UnmarshalJSON(b []byte) error {
	v := struct {
		Overlay   string
		Multiaddr string
	}{}
	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}

	a, err := swarm.ParseHexAddress(v.Overlay)
	if err != nil {
		return err
	}

	p.Overlay = a

	m, err := ma.NewMultiaddr(v.Multiaddr)
	if err != nil {
		return err
	}

	p.Multiaddr = m

	return nil
}
