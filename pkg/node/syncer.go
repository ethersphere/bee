// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"io"

	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/storer"
)

var errNoSyncProtocol = errors.New("full node needs at least one of --rangesync-enable and --puller-enable")

// syncMode is the historical-sync configuration of the node.
type syncMode struct {
	// Rangesync runs the rangesync protocol, the reconciler and the
	// reserve address index.
	Rangesync bool
	// Puller runs the puller (pulling via pullsync). The pullsync
	// protocol is registered regardless.
	Puller bool
	// RangesyncIgnored reports that --rangesync-enable was set on a node
	// that is not a full, non-boot node.
	RangesyncIgnored bool
}

// resolveSyncMode decides which historical-sync drivers run. Both apply only
// to full, non-boot nodes; elsewhere rangesync is ignored (RangesyncIgnored).
// A full node with both disabled is a configuration error.
func resolveSyncMode(fullNode, bootNode, rangesyncEnable, pullerEnable bool) (syncMode, error) {
	if !fullNode || bootNode {
		return syncMode{RangesyncIgnored: rangesyncEnable}, nil
	}
	if !rangesyncEnable && !pullerEnable {
		return syncMode{}, errNoSyncProtocol
	}
	return syncMode{Rangesync: rangesyncEnable, Puller: pullerEnable}, nil
}

var (
	_ storer.Syncer       = (*combinedSyncer)(nil)
	_ status.SyncReporter = (*combinedSyncer)(nil)
)

// combinedSyncer combines the enabled historical-sync drivers (puller,
// rangesync reconciler): Start starts all of them and SyncRate is the sum of
// their rates. With one driver it returns that driver's exact values.
type combinedSyncer struct {
	syncers []storer.Syncer
}

// newCombinedSyncer returns a combinedSyncer over the non-nil syncers.
func newCombinedSyncer(syncers ...storer.Syncer) *combinedSyncer {
	c := &combinedSyncer{}
	for _, s := range syncers {
		if s != nil {
			c.syncers = append(c.syncers, s)
		}
	}
	return c
}

// SyncRate returns the sum of the drivers' sync rates.
func (c *combinedSyncer) SyncRate() float64 {
	var r float64
	for _, s := range c.syncers {
		r += s.SyncRate()
	}
	return r
}

// Start starts every driver.
func (c *combinedSyncer) Start(ctx context.Context) {
	for _, s := range c.syncers {
		s.Start(ctx)
	}
}

// syncWiring returns what NewBee applies for mode: whether the storer opens
// the reserve address index, whether the rangesync service (with its
// reconciler and protocol) is built and registered, and whether the puller
// is built.
func syncWiring(mode syncMode) (addrIndex, registerRangesync, buildPuller bool) {
	return mode.Rangesync, mode.Rangesync, mode.Puller
}

// rangesyncCloser closes the reconciler before the rangesync service, so
// that no reconciler round calls a closed service.
type rangesyncCloser struct {
	reconciler io.Closer
	service    io.Closer
}

func (c rangesyncCloser) Close() error {
	return errors.Join(c.reconciler.Close(), c.service.Close())
}
