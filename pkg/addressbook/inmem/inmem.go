// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmem

import (
	"sync"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/swarm"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type inmem struct {
	mtx     sync.Mutex
	entries map[string]peerEntry // key: overlay in string value, value: peerEntry
}

type peerEntry struct {
	underlay libp2ppeer.ID // underlay address
	overlay  swarm.Address //overlay address
}

func New() addressbook.GetPutter {
	return &inmem{
		entries: make(map[string]peerEntry),
	}
}

func (i *inmem) Get(overlay swarm.Address) (underlay libp2ppeer.ID, exists bool) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	val, exists := i.entries[overlay.String()]
	return val.underlay, exists
}

func (i *inmem) Put(underlay libp2ppeer.ID, overlay swarm.Address) (exists bool) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	_, e := i.entries[overlay.String()]
	if e {
		return e // not sure if this is the right thing to do actually, maybe better to override? error?
	}

	i.entries[overlay.String()] = peerEntry{underlay: underlay, overlay: overlay}
	return e
}
