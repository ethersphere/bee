// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocklist

import (
	"errors"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
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
}

func (b *Blocklist) Exists(overlay swarm.Address) (bool, error) {
	key := generateKey(overlay)
	timestamp, duration, _, err := b.get(key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return false, nil
		}

		return false, err
	}

	if b.currentTimeFn().Sub(timestamp) > duration && duration != 0 {
		_ = b.store.Delete(key)
		return false, nil
	}

	return true, nil
}

func (b *Blocklist) Add(overlay swarm.Address, duration time.Duration, reason string) (err error) {
	key := generateKey(overlay)
	_, d, _, err := b.get(key)
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

		t, d, reason, err := b.get(string(k))
		if err != nil {
			return true, err
		}

		if b.currentTimeFn().Sub(t) > d && d != 0 {
			// skip to the next item
			return false, nil
		}

		p := p2p.BlockListedPeer{
			Peer: p2p.Peer{
				Address:  addr,
				FullNode: false,
			},
			Duration: d,
			Reason:   reason,
		}
		peers = append(peers, p)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return peers, nil
}

func (b *Blocklist) get(key string) (timestamp time.Time, duration time.Duration, reason string, err error) {
	var e entry
	if err := b.store.Get(key, &e); err != nil {
		return time.Time{}, -1, "", err
	}

	duration, err = time.ParseDuration(e.Duration)
	if err != nil {
		return time.Time{}, -1, "", err
	}

	return e.Timestamp, duration, e.Reason, nil
}

func generateKey(overlay swarm.Address) string {
	return keyPrefix + overlay.String()
}

func unmarshalKey(s string) (swarm.Address, error) {
	addr := s[len(keyPrefix):] // trim prefix
	return swarm.ParseHexAddress(addr)
}
