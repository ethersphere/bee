// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocklist

import (
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var keyPrefix = "blocklist-"

// timeNow is used to deterministically mock time.Now() in tests.
var timeNow = time.Now

type Blocklist struct {
	store storage.StateStorer
}

func NewBlocklist(store storage.StateStorer) *Blocklist {
	return &Blocklist{store: store}
}

type entry struct {
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"` // Duration is string because the time.Duration does not implement MarshalJSON/UnmarshalJSON methods.
}

func (b *Blocklist) Exists(overlay swarm.Address) (bool, error) {
	key := generateKey(overlay)
	timestamp, duration, err := b.get(key)
	if err != nil {
		if err == storage.ErrNotFound {
			return false, nil
		}

		return false, err
	}

	// using timeNow.Sub() so it can be mocked in unit tests
	if timeNow().Sub(timestamp) > duration && duration != 0 {
		_ = b.store.Delete(key)
		return false, nil
	}

	return true, nil
}

func (b *Blocklist) Add(overlay swarm.Address, duration time.Duration) (err error) {
	key := generateKey(overlay)
	_, d, err := b.get(key)
	if err != nil {
		if err != storage.ErrNotFound {
			return err
		}
	}

	// if peer is already blacklisted, blacklist it for the maximum amount of time
	if duration < d && duration != 0 || d == 0 {
		duration = d
	}

	return b.store.Put(key, &entry{
		Timestamp: timeNow(),
		Duration:  duration.String(),
	})
}

func (b *Blocklist) get(key string) (timestamp time.Time, duration time.Duration, err error) {
	var e entry
	if err := b.store.Get(key, &e); err != nil {
		return time.Time{}, -1, err
	}

	duration, err = time.ParseDuration(e.Duration)
	if err != nil {
		return time.Time{}, -1, err
	}

	return e.Timestamp, duration, nil
}

func generateKey(overlay swarm.Address) string {
	return keyPrefix + overlay.String()
}
