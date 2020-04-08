// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook

import (
	"encoding/json"

	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

type GetPutter interface {
	Getter
	Putter
}

type Getter interface {
	Get(overlay swarm.Address) (addr ma.Multiaddr, err error)
}

type Putter interface {
	Put(overlay swarm.Address, addr ma.Multiaddr) (exists bool, err error)
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
