// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var _ storer.Reserve = (*SimReserve)(nil)

// PutSource describes how a chunk entered the reserve.
type PutSource string

const (
	// PutSourceSync marks a chunk stored via the pullsync client (ReservePutter).
	PutSourceSync PutSource = "sync"
	// PutSourceInject marks a chunk stored via Inject (origin seeding).
	PutSourceInject PutSource = "inject"
)

// PutEvent is emitted, outside the reserve lock, whenever a new chunk is
// stored. The owning node index is attached by the caller wiring the hook.
type PutEvent struct {
	Address swarm.Address
	Bin     uint8
	BinID   uint64
	// BatchID and StampHash complete the (address, batchID, stampHash) triple
	// pullsync wants on. The deficit tracker strikes entries off on that triple,
	// so it must be able to see it here.
	BatchID     []byte
	StampHash   []byte
	Source      PutSource
	ReserveSize int
}

// Entry identifies one stored chunk by the triple pullsync wants on.
type Entry struct {
	Address   swarm.Address
	BatchID   []byte
	StampHash []byte
}

// binEntry is one append-only record in a bin log, ordered by BinID.
type binEntry struct {
	address   swarm.Address
	binID     uint64
	batchID   []byte
	stampHash []byte
}

// SimReserve is an in-memory storer.Reserve. It records chunk presence per
// node and feeds the real pullsync Syncer via SubscribeBin. It mints
// monotonically increasing per-bin BinIDs (starting at 1) so cursors and
// historical syncing behave as they do against the real reserve.
type SimReserve struct {
	base  swarm.Address
	bins  uint8
	epoch uint64 // build timestamp, always nonzero

	mu        sync.Mutex
	radius    uint8
	binLogs   [][]binEntry
	lastBinID []uint64
	presence  map[string]swarm.Chunk // key: addr.ByteString()+batchID+stampHash
	byAddr    map[string]int         // addr.ByteString() -> presence count
	subs      map[uint8][]chan struct{}

	quit     chan struct{}
	quitOnce sync.Once

	onPut func(PutEvent)
}

// NewSimReserve builds a reserve for a node based at base, with the given
// number of bins, initial radius, and epoch (must be nonzero). onPut may be
// nil; when set it is invoked outside the lock for every newly stored chunk.
func NewSimReserve(base swarm.Address, bins, radius uint8, epoch uint64, onPut func(PutEvent)) *SimReserve {
	return &SimReserve{
		base:      base,
		bins:      bins,
		epoch:     epoch,
		radius:    radius,
		binLogs:   make([][]binEntry, bins),
		lastBinID: make([]uint64, bins),
		presence:  make(map[string]swarm.Chunk),
		byAddr:    make(map[string]int),
		subs:      make(map[uint8][]chan struct{}),
		quit:      make(chan struct{}),
		onPut:     onPut,
	}
}

func presenceKey(addr swarm.Address, batchID, stampHash []byte) string {
	return addr.ByteString() + string(batchID) + string(stampHash)
}

// binOf returns the bin a chunk address lands in, capped at bins-1 so that
// with a reduced bin count every chunk still falls into a bin the puller syncs.
func (r *SimReserve) binOf(addr swarm.Address) uint8 {
	po := swarm.Proximity(r.base.Bytes(), addr.Bytes())
	if po > r.bins-1 {
		po = r.bins - 1
	}
	return po
}

// put stores a chunk idempotently. Storing a chunk that is already present is a
// no-op (returns nil) so that the same chunk arriving from several peers does
// not re-mint BinIDs and trigger duplicate offers.
func (r *SimReserve) put(ch swarm.Chunk, source PutSource) error {
	stamp := ch.Stamp()
	if stamp == nil {
		return storage.ErrInvalidChunk
	}
	batchID := stamp.BatchID()
	stampHash, err := stamp.Hash()
	if err != nil {
		return err
	}
	key := presenceKey(ch.Address(), batchID, stampHash)

	r.mu.Lock()
	if _, ok := r.presence[key]; ok {
		r.mu.Unlock()
		return nil
	}
	bin := r.binOf(ch.Address())
	r.lastBinID[bin]++
	binID := r.lastBinID[bin]
	r.binLogs[bin] = append(r.binLogs[bin], binEntry{
		address:   ch.Address(),
		binID:     binID,
		batchID:   batchID,
		stampHash: stampHash,
	})
	r.presence[key] = ch
	r.byAddr[ch.Address().ByteString()]++
	size := len(r.presence)
	r.triggerBin(bin)
	r.mu.Unlock()

	if r.onPut != nil {
		r.onPut(PutEvent{
			Address:     ch.Address(),
			Bin:         bin,
			BinID:       binID,
			BatchID:     batchID,
			StampHash:   stampHash,
			Source:      source,
			ReserveSize: size,
		})
	}
	return nil
}

// ReservePutter returns a putter that stores delivered chunks (source "sync").
func (r *SimReserve) ReservePutter() storage.Putter {
	return storage.PutterFunc(func(_ context.Context, ch swarm.Chunk) error {
		return r.put(ch, PutSourceSync)
	})
}

// Inject stores a chunk as origin-seeded content (source "inject").
func (r *SimReserve) Inject(ch swarm.Chunk) error {
	return r.put(ch, PutSourceInject)
}

func (r *SimReserve) ReserveGet(_ context.Context, addr swarm.Address, batchID, stampHash []byte) (swarm.Chunk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch, ok := r.presence[presenceKey(addr, batchID, stampHash)]; ok {
		return ch, nil
	}
	return nil, storage.ErrNotFound
}

func (r *SimReserve) ReserveHas(addr swarm.Address, batchID, stampHash []byte) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.presence[presenceKey(addr, batchID, stampHash)]
	return ok, nil
}

// SubscribeBin feeds every chunk in bin with BinID >= start, then blocks on the
// per-bin trigger for newly stored chunks. Closing the out channel signals
// end-of-page to the pullsync server (collectAddrs), so shutdown via Close
// unblocks parked handlers.
func (r *SimReserve) SubscribeBin(ctx context.Context, bin uint8, start uint64) (<-chan *storer.BinC, func(), <-chan error) {
	out := make(chan *storer.BinC)
	errC := make(chan error, 1)
	done := make(chan struct{})

	trigger, unsub := r.subscribe(bin)

	go func() {
		defer unsub()
		defer close(out)

		for {
			r.mu.Lock()
			var batch []binEntry
			if int(bin) < len(r.binLogs) {
				for _, e := range r.binLogs[bin] {
					if e.binID >= start {
						batch = append(batch, e)
					}
				}
			}
			r.mu.Unlock()

			for _, e := range batch {
				select {
				case out <- &storer.BinC{Address: e.address, BinID: e.binID, BatchID: e.batchID, StampHash: e.stampHash}:
					start = e.binID + 1
				case <-done:
					return
				case <-r.quit:
					return
				case <-ctx.Done():
					errC <- ctx.Err()
					return
				}
			}

			select {
			case <-trigger:
			case <-done:
				return
			case <-r.quit:
				return
			case <-ctx.Done():
				errC <- ctx.Err()
				return
			}
		}
	}()

	var doneOnce sync.Once
	return out, func() { doneOnce.Do(func() { close(done) }) }, errC
}

func (r *SimReserve) ReserveLastBinIDs() ([]uint64, uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]uint64, r.bins)
	copy(out, r.lastBinID)
	return out, r.epoch, nil
}

// IsWithinStorageRadius reports whether addr is at or beyond the storage radius.
func (r *SimReserve) IsWithinStorageRadius(addr swarm.Address) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return swarm.Proximity(r.base.Bytes(), addr.Bytes()) >= r.radius
}

func (r *SimReserve) StorageRadius() uint8 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.radius
}

// CommittedDepth mirrors the storage radius; neither pullsync nor puller reads
// it in this simulator, but the interface requires it.
func (r *SimReserve) CommittedDepth() uint8 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.radius
}

// CapacityDoubling is always zero here (no capacity doubling simulated).
func (r *SimReserve) CapacityDoubling() uint8 { return 0 }

// SetRadius updates the storage radius. A decrease exercises the puller's
// resetIntervals resync path.
func (r *SimReserve) SetRadius(radius uint8) {
	r.mu.Lock()
	r.radius = radius
	r.mu.Unlock()
}

func (r *SimReserve) EvictBatch(_ context.Context, _ []byte) error { return nil }

func (r *SimReserve) ReserveSample(context.Context, []byte, uint8, uint64, *big.Int) (storer.Sample, error) {
	return storer.Sample{}, nil
}

func (r *SimReserve) ReserveSize() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.presence)
}

// Close stops all subscription goroutines, unblocking any parked handlers.
func (r *SimReserve) Close() error {
	r.quitOnce.Do(func() { close(r.quit) })
	return nil
}

// subscribe registers a per-bin trigger channel and returns an unsubscribe fn.
func (r *SimReserve) subscribe(bin uint8) (<-chan struct{}, func()) {
	c := make(chan struct{}, 1)
	r.mu.Lock()
	r.subs[bin] = append(r.subs[bin], c)
	r.mu.Unlock()
	return c, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, s := range r.subs[bin] {
			if s == c {
				r.subs[bin] = append(r.subs[bin][:i], r.subs[bin][i+1:]...)
				break
			}
		}
	}
}

// triggerBin wakes all subscribers of a bin. Must be called under r.mu; sends
// are non-blocking on cap-1 channels so holding the lock is safe.
func (r *SimReserve) triggerBin(bin uint8) {
	for _, s := range r.subs[bin] {
		select {
		case s <- struct{}{}:
		default:
		}
	}
}

// Base returns the node's base address.
func (r *SimReserve) Base() swarm.Address { return r.base }

// HasAddress reports whether any chunk with the given address is stored,
// regardless of batch/stamp. Useful for tracking propagation of a specific
// chunk across nodes.
func (r *SimReserve) HasAddress(addr swarm.Address) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byAddr[addr.ByteString()] > 0
}

// Entries returns every stored chunk as an (address, batchID, stampHash)
// triple. It is the input to the deficit maths: the union of all surviving
// reserves' entries is the live chunk universe.
func (r *SimReserve) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, len(r.presence))
	for _, log := range r.binLogs {
		for _, e := range log {
			out = append(out, Entry{Address: e.address, BatchID: e.batchID, StampHash: e.stampHash})
		}
	}
	return out
}

// BinCounts returns a snapshot of the per-bin chunk counts.
func (r *SimReserve) BinCounts() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int, r.bins)
	for i := range r.binLogs {
		out[i] = len(r.binLogs[i])
	}
	return out
}
