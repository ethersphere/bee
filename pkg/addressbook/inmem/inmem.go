// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmem

import (
	"sync"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

type inmem struct {
	mtx     sync.Mutex
	entries map[string]peerEntry // key: overlay in string value, value: peerEntry
}

type peerEntry struct {
	overlay   swarm.Address //overlay address
	multiaddr ma.Multiaddr
}

func New() addressbook.GetPutter {
	return &inmem{
		entries: make(map[string]peerEntry),
	}
}

func (i *inmem) Get(overlay swarm.Address) (addr ma.Multiaddr, exists bool) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	val, exists := i.entries[overlay.String()]
	return val.multiaddr, exists
}

func (i *inmem) Put(overlay swarm.Address, addr ma.Multiaddr) (exists bool) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	_, e := i.entries[overlay.String()]
	if e {
		return e // not sure if this is the right thing to do actually, maybe better to override? error?
	}

	i.entries[overlay.String()] = peerEntry{overlay: overlay, multiaddr: addr}
	return e
}
