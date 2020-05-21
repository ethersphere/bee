// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tag

import (
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

// State is the enum type for chunk states
type State = uint32

const (
	StateStored State = iota // chunk has been processed by filehasher/swarm safe call
	StateSent                // chunk sent to neighbourhood
	StateSynced              // proof is received; chunk removed from sync db; chunk is available everywhere
)

var (
	errExists      = errors.New("already exists")
	errTagNotFound = errors.New("tag not found")
)

// Tag represents info on the status of new chunks
type Tag struct {
	Uid       uint32        // a unique identifier for this tag
	Stored    int64         // number of chunks already stored locally
	Sent      int64         // number of chunks sent for push syncing
	Synced    int64         // number of chunks synced with proof
	StartedAt time.Time     // tag started to calculate ETA
	Address   swarm.Address // the associated swarm hash for this tag
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NewTag creates a new tag, and returns it
func NewTag() *Tag {
	t := &Tag{
		Uid:       rand.Uint32(),
		StartedAt: time.Now(),
	}
	return t
}

// Inc increments the count for a state by 1
func (t *Tag) Inc(state State) {
	var v *int64
	switch state {
	case StateStored:
		v = &t.Stored
	case StateSent:
		v = &t.Sent
	case StateSynced:
		v = &t.Synced
	}
	atomic.AddInt64(v, int64(1))
}

// Get returns the count for a state on a tag
func (t *Tag) Get(state State) int64 {
	var v *int64
	switch state {
	case StateStored:
		v = &t.Stored
	case StateSent:
		v = &t.Sent
	case StateSynced:
		v = &t.Synced
	}
	return atomic.LoadInt64(v)
}
