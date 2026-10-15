// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// defaultAddrIndexBatchSize is the number of address index entries written
// or deleted per transaction by the fill and the drop.
const defaultAddrIndexBatchSize = 10_000

// elementIDSize is the size of an address index element ID:
// address ‖ stamp hash.
const elementIDSize = 2 * swarm.HashSize

var errInvalidAddrIndexKey = errors.New("reserve: invalid address index key")

// WithAddrIndexBatchSize sets the number of entries per fill transaction.
// It is meant for tests.
func WithAddrIndexBatchSize(n int) Option {
	return func(r *Reserve) { r.addrBatchSize = n }
}

// WithAddrIndexFillHook sets a function FillAddrIndex calls after each
// committed transaction with the number of entries filled so far. It is meant
// for tests, to interrupt a fill.
func WithAddrIndexFillHook(hook func(filled int)) Option {
	return func(r *Reserve) { r.addrFillHook = hook }
}

// addrIndexElementID returns the element ID (address ‖ stamp hash) of a
// ChunkAddrItem iteration result ID. Stores differ: inmemstore returns the
// bare 64-byte ID, leveldbstore returns "/" + ID with PrefixAtStart. Anything
// else is an error.
func addrIndexElementID(id string) (string, error) {
	switch {
	case len(id) == elementIDSize:
		return id, nil
	case len(id) == elementIDSize+1 && id[0] == '/':
		return id[1:], nil
	}
	return "", fmt.Errorf("%w: %d bytes", errInvalidAddrIndexKey, len(id))
}

// maskedPrefix returns the first ceil(bits/8) bytes of prefix with the bits
// past bits zeroed; missing prefix bytes are zero.
func maskedPrefix(prefix []byte, bits uint8) []byte {
	n := (int(bits) + 7) / 8
	out := make([]byte, n)
	copy(out, prefix)
	if r := bits % 8; r != 0 {
		out[n-1] &= 0xff << (8 - r)
	}
	return out
}

// hasMaskedPrefix reports whether the first bits bits of addr equal masked
// (as returned by maskedPrefix).
func hasMaskedPrefix(addr, masked []byte, bits uint8) bool {
	full := int(bits) / 8
	if !bytes.Equal(addr[:full], masked[:full]) {
		return false
	}
	if r := bits % 8; r != 0 {
		mask := byte(0xff << (8 - r))
		return addr[full]&mask == masked[full]
	}
	return true
}

// IterateAddr iterates the address index of st as described on
// storer.ReserveAddrStore.ReserveIterateAddr: the elements whose address
// starts with the first bits bits of prefix, in ascending order of
// address ‖ stamp hash, strictly after after (nil: from the range start).
//
// It is a single Iterate call: on leveldbstore one iterator, so one snapshot,
// opened before any callback without taking any lock of the reserve, and
// positioned by one seek at the masked range start (or at after, inside the
// range); the iteration ends at the first key past the range. On inmemstore
// the store walks its keys up to the seek position and holds its read lock
// across cb; the storer always runs on leveldbstore.
func IterateAddr(st storage.Reader, prefix swarm.Address, bits uint8, after []byte, cb func(ChunkID) (stop bool, err error)) error {
	masked := maskedPrefix(prefix.Bytes(), bits)
	seek := masked
	skipAfter := false
	if after != nil && bytes.Compare(after, masked) >= 0 {
		seek = after
		skipAfter = true
	}

	first := true
	err := st.Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkAddrItem{} },
		Prefix:        string(seek),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		id, err := addrIndexElementID(res.ID)
		if err != nil {
			return true, err
		}
		if first {
			first = false
			if skipAfter && id == string(after) {
				return false, nil
			}
		}
		if !hasMaskedPrefix([]byte(id[:swarm.HashSize]), masked, bits) {
			return true, nil
		}
		item := res.Entry.(*ChunkAddrItem)
		return cb(ChunkID{
			Address:   swarm.NewAddress([]byte(id[:swarm.HashSize])),
			StampHash: []byte(id[swarm.HashSize:]),
			BatchID:   item.BatchID,
		})
	})
	if err != nil {
		return fmt.Errorf("iterate address index: %w", err)
	}
	return nil
}

// IterateAddr iterates the reserve's address index; see the package-level
// IterateAddr. It returns ErrAddrIndexDisabled when the index is disabled.
func (r *Reserve) IterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(ChunkID) (stop bool, err error)) error {
	if !r.addrIndex {
		return ErrAddrIndexDisabled
	}
	return IterateAddr(r.st.IndexStore(), prefix, bits, after, cb)
}

// GetByID returns the chunk of the element (addr, stampHash) with its stamp:
// it resolves the batch ID from the address index, then calls Get. It
// returns an error wrapping storage.ErrNotFound when the element is not in
// the index or was removed in between.
func (r *Reserve) GetByID(ctx context.Context, addr swarm.Address, stampHash []byte) (swarm.Chunk, error) {
	if !r.addrIndex {
		return nil, ErrAddrIndexDisabled
	}
	item := &ChunkAddrItem{Address: addr, StampHash: stampHash}
	if err := r.st.IndexStore().Get(item); err != nil {
		return nil, fmt.Errorf("get address index entry: %w", err)
	}
	ch, err := r.Get(ctx, addr, item.BatchID, stampHash)
	if err != nil {
		return nil, fmt.Errorf("get reserve chunk: %w", err)
	}
	return ch, nil
}

// HasID reports whether the element (addr, stampHash) is in the address
// index.
func (r *Reserve) HasID(addr swarm.Address, stampHash []byte) (bool, error) {
	if !r.addrIndex {
		return false, ErrAddrIndexDisabled
	}
	has, err := r.st.IndexStore().Has(&ChunkAddrItem{Address: addr, StampHash: stampHash})
	if err != nil {
		return false, fmt.Errorf("has address index entry: %w", err)
	}
	return has, nil
}

// AddrIndexReady reports whether the address index is enabled and complete:
// CheckAddrIndex found a valid marker, or a fill or WriteAddrIndexMarker
// wrote one. The flag is set only after the marker write returned.
func (r *Reserve) AddrIndexReady() bool {
	return r.addrIndex && r.addrReady.Load()
}

// ReadAddrIndexDigest reads the digest of the reserve state in st: first the
// epoch (0 when absent) and the last bin ID of every bin, then the number of
// BatchRadiusItems. The order makes a read torn by a concurrent write safe:
// a put landing between the reads leaves the recorded bin IDs behind the
// store's for good (bin IDs only grow within an epoch), so the digest never
// matches the store again; a removal between them is in the count and in
// the index alike. The count stops with ctx's error when ctx is done.
func ReadAddrIndexDigest(ctx context.Context, st storage.Reader) (AddrIndexDigest, error) {
	var d AddrIndexDigest

	epoch := &EpochItem{}
	switch err := st.Get(epoch); {
	case err == nil:
		d.Epoch = epoch.Timestamp
	case !errors.Is(err, storage.ErrNotFound):
		return d, fmt.Errorf("read epoch: %w", err)
	}

	for bin := range swarm.MaxBins {
		item := &BinItem{Bin: bin}
		switch err := st.Get(item); {
		case err == nil:
			d.BinIDs[bin] = item.BinID
		case !errors.Is(err, storage.ErrNotFound):
			return d, fmt.Errorf("read bin ID of bin %d: %w", bin, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return d, err
	}
	var count uint64
	err := st.Iterate(storage.Query{
		Factory:      func() storage.Item { return &BatchRadiusItem{} },
		ItemProperty: storage.QueryItemID,
	}, func(storage.Result) (bool, error) {
		count++
		if count%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return true, err
			}
		}
		return false, nil
	})
	if err != nil {
		return d, fmt.Errorf("count reserve items: %w", err)
	}
	d.Count = count
	return d, nil
}

// loadAddrIndexMarker returns the validity marker, or nil if there is none.
func loadAddrIndexMarker(st storage.Reader) (*ChunkAddrValidItem, error) {
	m := &ChunkAddrValidItem{}
	switch err := st.Get(m); {
	case errors.Is(err, storage.ErrNotFound):
		return nil, nil
	case errors.Is(err, errUnmarshalInvalidSize):
		// a damaged marker counts as missing, but must still be deleted
		return &ChunkAddrValidItem{Version: 0}, nil
	case err != nil:
		return nil, fmt.Errorf("read address index marker: %w", err)
	}
	return m, nil
}

// HasAddrIndexMarker reports whether a validity marker exists, valid or not.
func HasAddrIndexMarker(st storage.Reader) (bool, error) {
	has, err := st.Has(&ChunkAddrValidItem{})
	if err != nil {
		return false, fmt.Errorf("check address index marker: %w", err)
	}
	return has, nil
}

// deleteAddrIndexMarker deletes the marker if present.
func deleteAddrIndexMarker(ctx context.Context, st transaction.Storage) error {
	has, err := HasAddrIndexMarker(st.IndexStore())
	if err != nil || !has {
		return err
	}
	err = st.Run(ctx, func(s transaction.Store) error {
		return s.IndexStore().Delete(&ChunkAddrValidItem{})
	})
	if err != nil {
		return fmt.Errorf("delete address index marker: %w", err)
	}
	return nil
}

// hasAddrIndexEntries reports whether the address index has any entry.
func hasAddrIndexEntries(st storage.Reader) (bool, error) {
	var found bool
	err := st.Iterate(storage.Query{
		Factory:      func() storage.Item { return &ChunkAddrItem{} },
		ItemProperty: storage.QueryItemID,
	}, func(storage.Result) (bool, error) {
		found = true
		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("check address index entries: %w", err)
	}
	return found, nil
}

// DisableAddrIndex is the start-up step of a storer whose address index is
// disabled: it deletes the validity marker if there is one and reports
// whether index entries are left to drop. With neither present it writes
// nothing (a leveldb delete of an absent key still writes a tombstone).
func DisableAddrIndex(ctx context.Context, st transaction.Storage) (leftovers bool, err error) {
	if err := deleteAddrIndexMarker(ctx, st); err != nil {
		return false, err
	}
	return hasAddrIndexEntries(st.IndexStore())
}

// DropAddrIndex deletes every address index entry, in transactions of a
// fixed size, and returns how many it deleted. It does not touch the marker.
// It stops with ctx's error when ctx is done.
func DropAddrIndex(ctx context.Context, st transaction.Storage) (int, error) {
	return dropAddrIndex(ctx, st, defaultAddrIndexBatchSize)
}

// dropAddrIndex deletes the entries one sub-range (first address byte) at a
// time: it reads the IDs of a sub-range with one iterator, then deletes them
// in transactions of batchSize. Reopening an iterator positioned after
// deleted keys would make leveldb skip their tombstones again on every page
// (PrefixAtStart seeks and steps back over them), which is quadratic in the
// size of a full index.
func dropAddrIndex(ctx context.Context, st transaction.Storage, batchSize int) (int, error) {
	var dropped int
	for first := range 256 {
		if err := ctx.Err(); err != nil {
			return dropped, err
		}
		var ids []string
		prefix := string([]byte{byte(first)})
		err := st.IndexStore().Iterate(storage.Query{
			Factory:      func() storage.Item { return &ChunkAddrItem{} },
			Prefix:       prefix,
			ItemProperty: storage.QueryItemID,
		}, func(res storage.Result) (bool, error) {
			// leveldbstore trims the query prefix from the ID, inmemstore
			// does not
			id := res.ID
			if len(id) == elementIDSize-len(prefix) {
				id = prefix + id
			}
			if len(id) != elementIDSize {
				return true, fmt.Errorf("%w: %d bytes", errInvalidAddrIndexKey, len(res.ID))
			}
			ids = append(ids, id)
			return false, nil
		})
		if err != nil {
			return dropped, fmt.Errorf("iterate address index: %w", err)
		}
		for len(ids) > 0 {
			if err := ctx.Err(); err != nil {
				return dropped, err
			}
			page := ids[:min(batchSize, len(ids))]
			ids = ids[len(page):]
			err = st.Run(ctx, func(s transaction.Store) error {
				for _, id := range page {
					err := s.IndexStore().Delete(&ChunkAddrItem{
						Address:   swarm.NewAddress([]byte(id[:swarm.HashSize])),
						StampHash: []byte(id[swarm.HashSize:]),
					})
					if err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return dropped, fmt.Errorf("delete address index entries: %w", err)
			}
			dropped += len(page)
		}
	}
	return dropped, nil
}

// AddrIndexKeyRange returns the leveldb key range of the address index
// entries, for compacting it after a drop.
func AddrIndexKeyRange() util.Range {
	ns := (&ChunkAddrItem{}).Namespace()
	return util.Range{Start: []byte(ns + "/"), Limit: []byte(ns + "0")} // '0' follows '/'
}

// CheckAddrIndex is the start-up step of an enabled address index. It must
// run before anything writes to the reserve. If the validity marker has the
// current version and the digest of the reserve, the index is ready and
// nothing is written. Otherwise it deletes the marker and drops every entry,
// synchronously, and reports that FillAddrIndex must follow; dropped is the
// number of entries deleted, so the caller can compact the key range.
func (r *Reserve) CheckAddrIndex(ctx context.Context) (needsFill bool, dropped int, err error) {
	if !r.addrIndex {
		return false, 0, ErrAddrIndexDisabled
	}
	m, err := loadAddrIndexMarker(r.st.IndexStore())
	if err != nil {
		return false, 0, err
	}
	if m != nil && m.Version == ChunkAddrIndexVersion {
		d, err := ReadAddrIndexDigest(ctx, r.st.IndexStore())
		if err != nil {
			return false, 0, err
		}
		if d == m.Digest {
			r.addrReady.Store(true)
			return false, 0, nil
		}
	}
	if m != nil {
		if err := deleteAddrIndexMarker(ctx, r.st); err != nil {
			return false, 0, err
		}
	}
	dropped, err = dropAddrIndex(ctx, r.st, r.addrBatchSize)
	if err != nil {
		return false, dropped, fmt.Errorf("drop address index: %w", err)
	}
	return true, dropped, nil
}

// FillAddrIndex fills the address index from the reserve's ChunkBinItems,
// then writes the validity marker and makes the index ready. It is the
// blocking fill: it must run after CheckAddrIndex reported needsFill and
// before anything else writes to the reserve, so it takes no reserve lock.
// The ChunkBinItems are read page by page (a new snapshot per page) and each
// page is written in one transaction. An interrupted fill (ctx done) writes
// no marker; the next start drops what it wrote and fills again. It returns
// the number of entries written.
//
// A background fill would run the same loop with a per-element step that
// takes the element's batch lock and re-checks its BatchRadiusItem in the
// transaction that writes the entry.
func (r *Reserve) FillAddrIndex(ctx context.Context) (int, error) {
	if !r.addrIndex {
		return 0, ErrAddrIndexDisabled
	}
	var (
		filled int
		cursor string // the ID of the last ChunkBinItem written
	)
	for {
		if err := ctx.Err(); err != nil {
			return filled, err
		}
		page := make([]*ChunkBinItem, 0, r.addrBatchSize)
		q := storage.Query{Factory: func() storage.Item { return &ChunkBinItem{} }}
		if cursor != "" {
			q.Prefix = cursor
			q.PrefixAtStart = true
		}
		err := r.st.IndexStore().Iterate(q, func(res storage.Result) (bool, error) {
			item := res.Entry.(*ChunkBinItem)
			if item.ID() == cursor {
				return false, nil
			}
			page = append(page, item)
			return len(page) == r.addrBatchSize, nil
		})
		if err != nil {
			return filled, fmt.Errorf("iterate chunk bin items: %w", err)
		}
		if len(page) == 0 {
			break
		}
		err = r.st.Run(ctx, func(s transaction.Store) error {
			for _, item := range page {
				err := s.IndexStore().Put(&ChunkAddrItem{
					Address:   item.Address,
					StampHash: item.StampHash,
					BatchID:   item.BatchID,
					ChunkType: item.ChunkType,
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return filled, fmt.Errorf("write address index entries: %w", err)
		}
		filled += len(page)
		cursor = page[len(page)-1].ID()
		if r.addrFillHook != nil {
			r.addrFillHook(filled)
		}
	}
	if err := r.WriteAddrIndexMarker(ctx); err != nil {
		return filled, err
	}
	return filled, nil
}

// WriteAddrIndexMarker reads the digest of the reserve (ReadAddrIndexDigest,
// bounded by ctx), writes the validity marker with it and then makes the
// index ready. The storer calls it when a fill completes, at a clean Close
// and after a reserve reset. The caller must know the index to be exact.
func (r *Reserve) WriteAddrIndexMarker(ctx context.Context) error {
	if !r.addrIndex {
		return ErrAddrIndexDisabled
	}
	d, err := ReadAddrIndexDigest(ctx, r.st.IndexStore())
	if err != nil {
		return fmt.Errorf("address index marker: %w", err)
	}
	err = r.st.Run(ctx, func(s transaction.Store) error {
		return s.IndexStore().Put(&ChunkAddrValidItem{Version: ChunkAddrIndexVersion, Digest: d})
	})
	if err != nil {
		return fmt.Errorf("write address index marker: %w", err)
	}
	r.addrReady.Store(true)
	return nil
}
