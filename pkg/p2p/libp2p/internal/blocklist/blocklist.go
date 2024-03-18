// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocklist

import (
	"errors"
	"strings"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var keyPrefix = "blocklist-"

type currentTimeFn = func() time.Time

type Blocklist struct {
	store         storage.StateStorer
	currentTimeFn currentTimeFn
}

func NewBlocklist(store storage.StateStorer) *Blocklist {
	return &Blocklist{
		store:         store,
		currentTimeFn: time.Now,
	}
}

type entry struct {
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"` // Duration is string because the time.Duration does not implement MarshalJSON/UnmarshalJSON methods.
	Reason    string    `json:"reason"`
	Full      bool      `json:"full"`
}

func (b *Blocklist) Exists(overlay swarm.Address) (bool, error) {
	key := generateKey(overlay)
	e, duration, err := b.get(key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return false, nil
		}

		return false, err
	}

	if b.currentTimeFn().Sub(e.Timestamp) > duration && duration != 0 {
		_ = b.store.Delete(key)
		return false, nil
	}

	return true, nil
}

func (b *Blocklist) Add(overlay swarm.Address, duration time.Duration, reason string, full bool) (err error) {
	key := generateKey(overlay)
	_, d, err := b.get(key)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
	}

	// if peer is already blacklisted, blacklist it for the maximum amount of time
	if duration < d && duration != 0 || d == 0 {
		duration = d
	}

	return b.store.Put(key, &entry{
		Timestamp: b.currentTimeFn(),
		Duration:  duration.String(),
		Reason:    reason,
		Full:      full,
	})
}

// Peers returns all currently blocklisted peers.
func (b *Blocklist) Peers() ([]p2p.BlockListedPeer, error) {
	var peers []p2p.BlockListedPeer
	if err := b.store.Iterate(keyPrefix, func(k, v []byte) (bool, error) {
		if !strings.HasPrefix(string(k), keyPrefix) {
			return true, nil
		}
		addr, err := unmarshalKey(string(k))
		if err != nil {
			return true, err
		}

		entry, d, err := b.get(string(k))
		if err != nil {
			return true, err
		}

		if b.currentTimeFn().Sub(entry.Timestamp) > d && d != 0 {
			// skip to the next item
			return false, nil
		}

		p := p2p.BlockListedPeer{
			Peer: p2p.Peer{
				Address:  addr,
				FullNode: entry.Full,
			},
			Duration: d,
			Reason:   entry.Reason,
		}
		peers = append(peers, p)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return peers, nil
}

func (b *Blocklist) get(key string) (entry, time.Duration, error) {
	var e entry
	err := b.store.Get(key, &e)
	if err != nil {
		return e, -1, err
	}

	dur, err := time.ParseDuration(e.Duration)
	if err != nil {
		return e, -1, err
	}

	return e, dur, nil
}

func generateKey(overlay swarm.Address) string {
	return keyPrefix + overlay.String()
}

func unmarshalKey(s string) (swarm.Address, error) {
	addr := s[len(keyPrefix):] // trim prefix
	return swarm.ParseHexAddress(addr)
}
