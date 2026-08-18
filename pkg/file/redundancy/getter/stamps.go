// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy/stampcarrier"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	errNoCarriers     = errors.New("getter: no stamp carriers")
	errNoCarrierEntry = errors.New("getter: no carrier entry for chunk")
)

// loadCarrierPayloads fetches the stamp carrier group of the intermediate
// chunk and reconstructs missing members via the carrier RS group (spec §7).
//
// The group members are fetched concurrently under a single FetchTimeout
// deadline, so the worst case is one chunk fetch timeout rather than one per
// member. This matters because readers of the intermediate chunk are released
// only once recover() - and with it stamp recovery - has finished.
//
// The whole group is always fetched, never just the one carrier holding the
// entry of interest: recover() typically rebuilds several shards at once, and
// a missing member has to be reconstructed from the others anyway. A lazy,
// per-carrier variant is out of scope for the PoC.
//
// The result is cached for the decoder's lifetime: a decoder is short lived
// and shared by the readers of one intermediate chunk, so a transient carrier
// retrieval failure is not retried within it - the shards it rebuilds are
// then saved unstamped, exactly as before stamp carriers existed.
func (g *decoder) loadCarrierPayloads() ([][]byte, error) {
	g.carrierOnce.Do(func() {
		if len(g.carrierAddrs) <= stampcarrier.GroupParities {
			g.carrierErr = errNoCarriers
			return
		}
		c := len(g.carrierAddrs) - stampcarrier.GroupParities

		ctx, cancel := context.WithTimeout(context.Background(), g.config.FetchTimeout)
		defer cancel()

		// each goroutine writes only its own index, the WaitGroup provides
		// the happens-before for the reads below
		shards := make([][]byte, len(g.carrierAddrs))
		var wg sync.WaitGroup
		for i, addr := range g.carrierAddrs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ch, err := g.fetcher.Get(ctx, addr)
				if err != nil || len(ch.Data()) != swarm.ChunkWithSpanSize {
					return
				}
				shards[i] = ch.Data()[swarm.SpanSize:]
			}()
		}
		wg.Wait()

		for _, shard := range shards {
			if shard == nil {
				if err := stampcarrier.ReconstructGroup(shards, c); err != nil {
					g.carrierErr = err
					return
				}
				break
			}
		}
		g.carrierPayloads = shards[:c]
	})
	return g.carrierPayloads, g.carrierErr
}

// recoverStamp recovers and validates the original stamp of the child at
// slot i. Any failure returns an error and the caller degrades to saving the
// chunk unstamped (spec constraint 6).
func (g *decoder) recoverStamp(i int) (swarm.Stamp, error) {
	payloads, err := g.loadCarrierPayloads()
	if err != nil {
		return nil, err
	}
	j := i / stampcarrier.MaxEntries
	if j >= len(payloads) {
		return nil, errNoCarrierEntry
	}
	entries, err := stampcarrier.Unpack(payloads[j])
	if err != nil {
		return nil, err
	}
	b, ok := entries[uint16(i)]
	if !ok {
		return nil, errNoCarrierEntry
	}
	stamp := new(postage.Stamp)
	if err := stamp.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	owner, err := g.config.BatchOwnerFn(stamp.BatchID())
	if err != nil {
		return nil, err
	}
	if err := stamp.ValidBinding(g.addrs[i], owner); err != nil {
		return nil, err
	}
	return stamp, nil
}

// recoverStamps recovers the original stamps of the given shard indexes.
// It must be called without holding the decoder's buffer lock: carrier
// retrieval goes through the network fetcher and must not block concurrent
// shard reads. Indexes whose stamp cannot be recovered are absent from the
// result and their chunks are saved unstamped.
//
// Without a batch owner resolver no stamp can be validated, so the carrier
// group is not fetched at all - the sole reason to retrieve it is to produce
// validated stamps.
func (g *decoder) recoverStamps(missing []int) map[int]swarm.Stamp {
	if g.config.BatchOwnerFn == nil {
		return nil
	}
	stamps := make(map[int]swarm.Stamp, len(missing))
	for _, i := range missing {
		stamp, err := g.recoverStamp(i)
		if err != nil {
			g.logger.Debug("stamp recovery failed", "chunk_address", g.addrs[i], "error", err)
			continue
		}
		stamps[i] = stamp
	}
	return stamps
}
