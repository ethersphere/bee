// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"container/list"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// sourceWindow is how long a peer that was denied an ID stays a source
	// of it (§7.8, "known this round"): the claim deadline, after which the
	// peer's own next session claims the ID again if it is still missing.
	sourceWindow = rangesync.ClaimDeadline
	// maxSourceIDs bounds the IDs wantSources keeps: as many as the want set
	// holds claims. A denied ID has an active claim when it is recorded.
	maxSourceIDs = wantset.MaxClaims
)

// resultFetcher is implemented by a Service whose fetches report which IDs
// the peer did not deliver and which another fetch's claim held
// (*rangesync.Service). With it, the reconciler retries undelivered IDs from
// other peers (§7.8) and judges stripe withholding on undelivered IDs only.
type resultFetcher interface {
	FetchWithResult(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (rangesync.FetchResult, error)
}

var _ resultFetcher = (*rangesync.Service)(nil)

// fetchOutcome is what one fetch from a peer achieved.
type fetchOutcome struct {
	stored int // chunks stored
	// undelivered is the number of IDs requested from the peer that it did
	// not deliver; without a resultFetcher, every ID not stored.
	undelivered int
}

// fetchFrom fetches ids from peer. With a resultFetcher it records peer as a
// source of the IDs another fetch's claim held (the want set denied them),
// and retries the IDs peer did not deliver from their other sources (retry).
// The retries of a catch-up fetch are catch-up fetches.
func (r *Reconciler) fetchFrom(ctx context.Context, peer swarm.Address, ids []storer.ChunkID, catchUp bool) (fetchOutcome, error) {
	rf, ok := r.svc.(resultFetcher)
	if !ok {
		n, err := r.svc.Fetch(ctx, peer, ids)
		return fetchOutcome{stored: n, undelivered: len(ids) - n}, err
	}
	res, err := rf.FetchWithResult(ctx, peer, ids)
	r.sources.add(r.now(), peer, res.Held)
	r.retry(peer, res.Undelivered, catchUp)
	return fetchOutcome{stored: res.Stored, undelivered: len(res.Undelivered)}, err
}

// retry fetches ids, which failed did not deliver (or delivered invalid, or
// whose fetch failed), from other peers known to hold them: the peers the
// want set denied them while failed's claim held them, within sourceWindow
// (wantSources). Each ID goes to the first such peer that is still
// connected, other than failed, and is dropped from its sources. The IDs of
// one peer are fetched in a goroutine of their own, one retry fetch per peer
// at a time, so that retries add at most one fetch stream per peer. A retry's
// own undelivered IDs go to the next source. IDs with no other source are
// left to the next round, which lists them again.
func (r *Reconciler) retry(failed swarm.Address, ids []storer.ChunkID, catchUp bool) {
	if len(ids) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil {
		return
	}
	known := func(p swarm.Address) bool {
		st, ok := r.peers[p.ByteString()]
		return ok && st.fsm.state != stateUnsupported
	}
	for _, b := range r.sources.take(r.now(), failed, ids, known) {
		if catchUp {
			r.catchUpFetches++ // the caller's fetch is still counted
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			release, err := r.retryLocks.acquire(r.ctx, b.peer)
			if err != nil {
				b.ids = nil // closing: only the catch-up count ends
			} else {
				defer release()
			}
			r.fetch(b.peer, b.ids, catchUp)
		}()
	}
}

// wantSources is the id → peers map of §7.8: for each ID the want set denied
// a fetch because another fetch held its claim, the peers of the denied
// fetches, which are known to hold the ID. When the claim's fetch fails to
// deliver the ID, retry requests it from one of them. An ID is kept for
// sourceWindow after it was first denied, at most maxSourceIDs IDs (the
// oldest are dropped first). It is safe for concurrent use.
type wantSources struct {
	mu    sync.Mutex
	byID  map[fingerprint.ID]*list.Element // values are *sourceEntry
	order *list.List                       // oldest first
}

// sourceEntry holds the peers denied one ID, in the order they were denied.
type sourceEntry struct {
	id    fingerprint.ID
	at    time.Time
	peers []swarm.Address
}

// retryBatch is the IDs to retry from one peer.
type retryBatch struct {
	peer swarm.Address
	ids  []storer.ChunkID
}

func newWantSources() *wantSources {
	return &wantSources{byID: make(map[fingerprint.ID]*list.Element), order: list.New()}
}

// add records peer as a source of ids, which the want set denied it at now.
func (w *wantSources) add(now time.Time, peer swarm.Address, ids []storer.ChunkID) {
	if len(ids) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.prune(now)
	for _, c := range ids {
		id := fingerprint.NewID(c.Address, c.StampHash)
		if e, ok := w.byID[id]; ok {
			se := e.Value.(*sourceEntry)
			if !slices.ContainsFunc(se.peers, peer.Equal) {
				se.peers = append(se.peers, peer)
			}
			continue
		}
		if w.order.Len() >= maxSourceIDs {
			w.remove(w.order.Front())
		}
		w.byID[id] = w.order.PushBack(&sourceEntry{id: id, at: now, peers: []swarm.Address{peer}})
	}
}

// take returns, grouped by peer in the order of first use, a source of each
// of ids other than failed that known accepts, the first in the order they
// were denied. It drops the sources it passes over or returns.
func (w *wantSources) take(now time.Time, failed swarm.Address, ids []storer.ChunkID, known func(swarm.Address) bool) []retryBatch {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.prune(now)
	var (
		out   []retryBatch
		index = make(map[string]int)
	)
	for _, c := range ids {
		e, ok := w.byID[fingerprint.NewID(c.Address, c.StampHash)]
		if !ok {
			continue
		}
		se := e.Value.(*sourceEntry)
		var (
			peer  swarm.Address
			found bool
		)
		for len(se.peers) > 0 && !found {
			p := se.peers[0]
			se.peers = se.peers[1:]
			found = !p.Equal(failed) && known(p)
			peer = p
		}
		if len(se.peers) == 0 {
			w.remove(e)
		}
		if !found {
			continue
		}
		i, ok := index[peer.ByteString()]
		if !ok {
			i = len(out)
			index[peer.ByteString()] = i
			out = append(out, retryBatch{peer: peer})
		}
		out[i].ids = append(out[i].ids, c)
	}
	return out
}

// prune drops the IDs first denied sourceWindow or more before now. w.mu
// must be held.
func (w *wantSources) prune(now time.Time) {
	for e := w.order.Front(); e != nil; e = w.order.Front() {
		if now.Sub(e.Value.(*sourceEntry).at) < sourceWindow {
			return
		}
		w.remove(e)
	}
}

// remove drops the ID held in e. w.mu must be held.
func (w *wantSources) remove(e *list.Element) {
	w.order.Remove(e)
	delete(w.byID, e.Value.(*sourceEntry).id)
}
