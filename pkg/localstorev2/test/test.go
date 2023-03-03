// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"context"
	"testing"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	batchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/pkg/pullsync"
	pullsyncMock "github.com/ethersphere/bee/pkg/pullsync/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	kademlia "github.com/ethersphere/bee/pkg/topology/mock"
)

func NewStorer(t *testing.T, opts *storer.Options) (*storer.DB, error) {
	t.Helper()
	return storer.New(context.Background(), "", opts)
}

func DBTestOps(baseAddr swarm.Address, reserveCapacity int, bs postage.Storer, syncer pullsync.SyncReporter, radiusSetter topology.SetStorageRadiuser, reserveWakeUpTime time.Duration) *storer.Options {

	opts := &storer.Options{}

	if radiusSetter == nil {
		radiusSetter = kademlia.NewTopologyDriver()
	}

	if bs == nil {
		bs = batchstore.New()
	}

	if syncer == nil {
		syncer = pullsyncMock.NewMockRateReporter(0)
	}

	opts.Address = baseAddr
	opts.RadiusSetter = radiusSetter
	opts.ReserveCapacity = reserveCapacity
	opts.Batchstore = bs
	opts.Syncer = syncer
	opts.ReserveWakeUpDuration = reserveWakeUpTime
	opts.Logger = log.Noop

	return opts
}
