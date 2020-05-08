// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrNotFound     = errors.New("storage: not found")
	ErrInvalidChunk = errors.New("storage: invalid chunk")
)

// ModeGet enumerates different Getter modes.
type ModeGet int

func (m ModeGet) String() string {
	switch m {
	case ModeGetRequest:
		return "Request"
	case ModeGetSync:
		return "Sync"
	case ModeGetLookup:
		return "Lookup"
	case ModeGetPin:
		return "PinLookup"
	default:
		return "Unknown"
	}
}

// Getter modes.
const (
	// ModeGetRequest: when accessed for retrieval
	ModeGetRequest ModeGet = iota
	// ModeGetSync: when accessed for syncing or proof of custody request
	ModeGetSync
	// ModeGetLookup: when accessed to lookup a a chunk in feeds or other places
	ModeGetLookup
	// ModeGetPin: used when a pinned chunk is accessed
	ModeGetPin
)

// ModePut enumerates different Putter modes.
type ModePut int

func (m ModePut) String() string {
	switch m {
	case ModePutRequest:
		return "Request"
	case ModePutSync:
		return "Sync"
	case ModePutUpload:
		return "Upload"
	default:
		return "Unknown"
	}
}

// Putter modes.
const (
	// ModePutRequest: when a chunk is received as a result of retrieve request and delivery
	ModePutRequest ModePut = iota
	// ModePutSync: when a chunk is received via syncing
	ModePutSync
	// ModePutUpload: when a chunk is created by local upload
	ModePutUpload
)

// ModeSet enumerates different Setter modes.
type ModeSet int

func (m ModeSet) String() string {
	switch m {
	case ModeSetAccess:
		return "Access"
	case ModeSetSyncPush:
		return "SyncPush"
	case ModeSetSyncPull:
		return "SyncPull"
	case ModeSetRemove:
		return "Remove"
	case ModeSetPin:
		return "ModeSetPin"
	case ModeSetUnpin:
		return "ModeSetUnpin"
	default:
		return "Unknown"
	}
}

// Setter modes.
const (
	// ModeSetAccess: when an update request is received for a chunk or chunk is retrieved for delivery
	ModeSetAccess ModeSet = iota
	// ModeSetSyncPush: when a push sync receipt is received for a chunk
	ModeSetSyncPush
	// ModeSetSyncPull: when a chunk is added to a pull sync batch
	ModeSetSyncPull
	// ModeSetRemove: when a chunk is removed
	ModeSetRemove
	// ModeSetPin: when a chunk is pinned during upload or separately
	ModeSetPin
	// ModeSetUnpin: when a chunk is unpinned using a command locally
	ModeSetUnpin
)

// Descriptor holds information required for Pull syncing. This struct
// is provided by subscribing to pull index.
type Descriptor struct {
	Address swarm.Address
	BinID   uint64
}

func (d *Descriptor) String() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("%s bin id %v", d.Address, d.BinID)
}

type Storer interface {
	Get(ctx context.Context, mode ModeGet, addr swarm.Address) (ch swarm.Chunk, err error)
	GetMulti(ctx context.Context, mode ModeGet, addrs ...swarm.Address) (ch []swarm.Chunk, err error)
	Put(ctx context.Context, mode ModePut, chs ...swarm.Chunk) (exist []bool, err error)
	Has(ctx context.Context, addr swarm.Address) (yes bool, err error)
	HasMulti(ctx context.Context, addrs ...swarm.Address) (yes []bool, err error)
	Set(ctx context.Context, mode ModeSet, addrs ...swarm.Address) (err error)
	LastPullSubscriptionBinID(bin uint8) (id uint64, err error)
	SubscribePull(ctx context.Context, bin uint8, since, until uint64) (c <-chan Descriptor, stop func())
	SubscribePush(ctx context.Context) (c <-chan swarm.Chunk, stop func())
	io.Closer
}

type Putter interface {
	Put(ctx context.Context, mode ModePut, chs ...swarm.Chunk) (exist []bool, err error)
}

// StateStorer defines methods required to get, set, delete values for different keys
// and close the underlying resources.
type StateStorer interface {
	Get(key string, i interface{}) (err error)
	Put(key string, i interface{}) (err error)
	Delete(key string) (err error)
	Iterate(prefix string, iterFunc StateIterFunc) (err error)
	io.Closer
}

// StateIterFunc is used when iterating through StateStorer key/value pairs
type StateIterFunc func(key, value []byte) (stop bool, err error)
