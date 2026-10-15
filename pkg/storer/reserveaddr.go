// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
)

const (
	// addrIndexFillBlockingLimit is how long the blocking address index fill
	// may take on a full reserve (proposal §5.1, plan decision D5); a longer
	// fill is logged as a warning.
	addrIndexFillBlockingLimit = time.Minute
	// addrIndexMarkerTimeout bounds the digest count and the marker write in
	// Close.
	addrIndexMarkerTimeout = 30 * time.Second
	// addrIndexStopTimeout bounds the wait for the background drop in Close.
	addrIndexStopTimeout = 5 * time.Second
)

// addrIndexMode is how New treats the reserve address index.
type addrIndexMode uint8

const (
	addrIndexOff  addrIndexMode = iota + 1 // delete the marker, drop the entries
	addrIndexOn                            // maintain the index
	addrIndexKeep                          // leave the marker and the entries alone
)

func addrIndexModeOf(opts *Options) addrIndexMode {
	switch {
	case opts.ReserveAddrIndex && opts.ReserveCapacity > 0:
		return addrIndexOn
	case opts.ReserveAddrIndex:
		return addrIndexOff // no reserve
	case opts.ReserveAddrIndexKeep:
		return addrIndexKeep
	default:
		return addrIndexOff
	}
}

// addrIndexState is the DB's address index state.
type addrIndexState struct {
	mode addrIndexMode
	// dropCancel and dropDone belong to the background drop started by New
	// with the index disabled; both are nil when none was started.
	dropCancel context.CancelFunc
	dropDone   chan struct{}
	// fills counts the fills New ran (for tests).
	fills int
}

// active reports whether Close must take the ordered path.
func (s *addrIndexState) active() bool {
	return s.mode == addrIndexOn || s.dropDone != nil
}

// startAddrIndex reconciles the address index with its marker (see
// Options.ReserveAddrIndex). It runs at the end of New, before anything else
// can write to the reserve.
func (db *DB) startAddrIndex(ctx context.Context, opts *Options) error {
	db.addrIndex.mode = addrIndexModeOf(opts)

	switch db.addrIndex.mode {
	case addrIndexOn:
		needsFill, dropped, err := db.reserve.CheckAddrIndex(ctx)
		if err != nil {
			return err
		}
		if dropped > 0 {
			db.logger.Debug("dropped stale reserve address index", "count", dropped)
			db.compactAddrIndex()
		}
		if !needsFill {
			return nil
		}
		db.addrIndex.fills++
		start := time.Now()
		db.logger.Info("filling the reserve address index, do not interrupt", "reserve_size", db.reserve.Size())
		filled, err := db.reserve.FillAddrIndex(ctx)
		if err != nil {
			return err
		}
		took := time.Since(start)
		db.logger.Info("filled the reserve address index", "count", filled, "duration", took)
		if took > addrIndexFillBlockingLimit {
			db.logger.Warning("reserve address index fill took longer than expected", "duration", took, "limit", addrIndexFillBlockingLimit)
		}
		return nil

	case addrIndexOff:
		leftovers, err := reserve.DisableAddrIndex(ctx, db.storage)
		if err != nil {
			return err
		}
		if leftovers {
			db.startAddrIndexDrop()
		}
		return nil
	}
	return nil
}

// startAddrIndexDrop drops the entries of a disabled address index in the
// background, then compacts their key range. Close stops it.
func (db *DB) startAddrIndexDrop() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	db.addrIndex.dropCancel = cancel
	db.addrIndex.dropDone = done

	go func() {
		defer close(done)
		dropped, err := reserve.DropAddrIndex(ctx, db.storage)
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, leveldb.ErrClosed):
			// ErrClosed: Close timed out waiting and closed the store
			db.logger.Debug("reserve address index drop interrupted; it resumes at the next start", "dropped", dropped)
			return
		case err != nil:
			db.logger.Warning("reserve address index drop failed; it is retried at the next start", "error", err)
			return
		}
		db.logger.Debug("dropped the disabled reserve address index", "count", dropped)
		db.compactAddrIndex()
	}()
}

// compactAddrIndex compacts the address index key range after a drop, so its
// tombstones don't slow down later seeks into the range.
func (db *DB) compactAddrIndex() {
	if db.pinIntegrity == nil {
		return // in-memory store
	}
	ldb, ok := db.pinIntegrity.Store.(leveldbstore.Storer)
	if !ok {
		return
	}
	if err := ldb.DB().CompactRange(reserve.AddrIndexKeyRange()); err != nil {
		db.logger.Debug("compact the reserve address index range", "error", err)
	}
}

// closeWithAddrIndex is Close with an enabled address index or a background
// drop running: stop the drop, wait for the workers, rewrite the marker, then
// close the store (see Close). The waits run one after the other, so Close
// can take up to about 48 s in the worst case: the drop (5 s), the workers
// (5 s each), the marker's digest count and write (30 s, ≈ 3 s at 4.2M items
// as measured for G4) and the store close (3 s).
func (db *DB) closeWithAddrIndex() error {
	waited := true

	if db.addrIndex.dropDone != nil {
		db.addrIndex.dropCancel()
		select {
		case <-db.addrIndex.dropDone:
		case <-time.After(addrIndexStopTimeout):
			db.logger.Warning("reserve address index drop still running at shutdown")
			waited = false
		}
	}

	if !syncutil.WaitWithTimeout(&db.inFlight, 5*time.Second) {
		db.logger.Warning("db shutting down with running goroutines")
		waited = false
	}
	if !syncutil.WaitWithTimeout(&db.cacheLimiter.wg, 5*time.Second) {
		db.logger.Warning("cache goroutines still running after the wait timeout; force closing")
		db.cacheLimiter.cancel()
		waited = false
	}

	if db.addrIndex.mode == addrIndexOn && waited && db.reserve.AddrIndexReady() {
		ctx, cancel := context.WithTimeout(context.Background(), addrIndexMarkerTimeout)
		err := db.reserve.WriteAddrIndexMarker(ctx)
		cancel()
		if err != nil {
			db.logger.Warning("reserve address index marker not written; the next start refills the index", "error", err)
		}
	}

	errC := make(chan error, 1)
	go func() { errC <- db.dbCloser.Close() }()
	select {
	case err := <-errC:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("storer closed with bg goroutines running")
	}
}

// ReserveIterateAddr implements ReserveAddrStore.
func (db *DB) ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(ChunkID) (stop bool, err error)) (err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveIterateAddr").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveIterateAddr", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveIterateAddr", "failure").Inc()
		}
	}()

	if db.reserve == nil {
		return ErrAddrIndexDisabled
	}
	return db.reserve.IterateAddr(prefix, bits, after, cb)
}

// ReserveGetByID implements ReserveAddrStore.
func (db *DB) ReserveGetByID(ctx context.Context, addr swarm.Address, stampHash []byte) (ch swarm.Chunk, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveGetByID").Observe(dur())
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveGetByID", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveGetByID", "failure").Inc()
			db.logger.Debug("reserve get by id error", "error", err)
		}
	}()

	if db.reserve == nil {
		return nil, ErrAddrIndexDisabled
	}
	return db.reserve.GetByID(ctx, addr, stampHash)
}

// ReserveHasID implements ReserveAddrStore.
func (db *DB) ReserveHasID(addr swarm.Address, stampHash []byte) (has bool, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveHasID").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHasID", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHasID", "failure").Inc()
			db.logger.Debug("reserve has id error", "error", err)
		}
	}()

	if db.reserve == nil {
		return false, ErrAddrIndexDisabled
	}
	return db.reserve.HasID(addr, stampHash)
}

// ReserveAddrIndexReady implements ReserveAddrStore.
func (db *DB) ReserveAddrIndexReady() bool {
	return db.reserve != nil && db.reserve.AddrIndexReady()
}

// SetReserveObserver implements ReserveAddrStore.
func (db *DB) SetReserveObserver(o ReserveObserver) error {
	if db.reserve == nil {
		return ErrAddrIndexDisabled
	}
	if err := db.reserve.SetObserver(o); err != nil {
		return fmt.Errorf("set reserve observer: %w", err)
	}
	return nil
}
