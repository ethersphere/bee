// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"io"

	"github.com/ethersphere/bee/v2/pkg/log"
)

var (
	ValidatePublicAddress = validatePublicAddress
	UseEmbeddedSnapshot   = useEmbeddedSnapshot
)

// NewTestBeeWithStatus returns a Bee with a status store for phase tests.
func NewTestBeeWithStatus() *Bee {
	return &Bee{
		logger: log.Noop,
		status: NewStatusStore(),
	}
}

func (b *Bee) ApplyReservePhase(phase string, syncRate func() float64, stabilized func() bool) {
	b.applyReservePhase(phase, syncRate, stabilized)
}

func (b *Bee) SetReadyFromWarmup() { b.setReadyFromWarmup() }

func (b *Bee) CurrentStatus() Status {
	return b.status.Status()
}

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
