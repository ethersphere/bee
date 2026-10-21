// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import "io"

var (
	ValidatePublicAddress = validatePublicAddress
	UseEmbeddedSnapshot   = useEmbeddedSnapshot
)

// NewTestBeeWithClosers builds a Bee with only the push-sync and retrieval
// closer fields set, for exercising the Shutdown closer registration.
func NewTestBeeWithClosers(pushSync, retrieval io.Closer) *Bee {
	return &Bee{
		pushSyncCloser:  pushSync,
		retrievalCloser: retrieval,
	}
}

// ShutdownClosersByName returns the Shutdown fan-out closers keyed by name.
func (b *Bee) ShutdownClosersByName() map[string]io.Closer {
	m := make(map[string]io.Closer)
	for _, nc := range b.shutdownClosers() {
		m[nc.name] = nc.closer
	}
	return m
}

var (
	NewCombinedSyncer = newCombinedSyncer
	ResolveSyncMode   = resolveSyncMode
	ErrNoSyncProtocol = errNoSyncProtocol
)

var (
	SyncWiring       = syncWiring
	RangesyncOptions = rangesyncOptions
)

func NewRangesyncCloser(reconciler, service io.Closer) io.Closer {
	return rangesyncCloser{reconciler: reconciler, service: service}
}

func ResolveSyncWiring(full, boot, rs, pull bool) (addrIndex, registerRangesync, buildPuller bool, err error) {
	m, err := resolveSyncMode(full, boot, rs, pull)
	if err != nil {
		return false, false, false, err
	}
	a, r, p := syncWiring(m)
	return a, r, p, nil
}
