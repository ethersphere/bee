// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	// errMarshalInvalidRetrievalIndexAddress is returned if the RetrievalIndexItem address is zero during marshaling.
	errMarshalInvalidRetrievalIndexAddress = errors.New("marshal RetrievalIndexItem: address is zero")
	// errMarshalInvalidRetrievalIndexLocation is returned if the RetrievalIndexItem location is invalid during marshaling.
	errMarshalInvalidRetrievalIndexLocation = errors.New("marshal RetrievalIndexItem: location is invalid")
	// errUnmarshalInvalidRetrievalIndexSize is returned during unmarshaling if the passed buffer is not the expected size.
	errUnmarshalInvalidRetrievalIndexSize = errors.New("unmarshal RetrievalIndexItem: invalid size")
	// errUnmarshalInvalidRetrievalIndexLocationBytes is returned during unmarshaling if the location buffer is invalid.
	errUnmarshalInvalidRetrievalIndexLocationBytes = errors.New("unmarshal RetrievalIndexItem: invalid location bytes")
)

const RetrievalIndexItemSize = swarm.HashSize + 8 + sharky.LocationSize + 4

var _ storage.Item = (*RetrievalIndexItem)(nil)

// Sharky provides an abstraction for the sharky.Store operations used in the
// chunkstore. This allows us to be more flexible in passing in the sharky instance
// to chunkstore. For eg, check the TxChunkStore implementation in this pkg.
type Sharky interface {
	Read(context.Context, sharky.Location, []byte) error
	Write(context.Context, []byte) (sharky.Location, error)
	Release(context.Context, sharky.Location) error
}

func Get(ctx context.Context, r storage.Reader, s storage.Sharky, addr swarm.Address) (swarm.Chunk, error) {
	rIdx := &RetrievalIndexItem{Address: addr}
	err := r.Get(rIdx)
	if err != nil {
		return nil, fmt.Errorf("chunk store: failed reading retrievalIndex for address %s: %w", addr, err)
	}
	return readChunk(ctx, s, rIdx)
}

// GetInto reads chunk data into the provided buffer, avoiding per-call allocation.
// Returns the number of bytes read.
func GetInto(ctx context.Context, r storage.Reader, s storage.Sharky, addr swarm.Address, buf []byte) (int, error) {
	rIdx := &RetrievalIndexItem{Address: addr}
	err := r.Get(rIdx)
	if err != nil {
		return 0, fmt.Errorf("chunk store: failed reading retrievalIndex for address %s: %w", addr, err)
	}
	n := int(rIdx.Location.Length)
	if len(buf) < n {
		return 0, fmt.Errorf("chunk store: buffer too small: %d < %d", len(buf), n)
	}
	err = s.Read(ctx, rIdx.Location, buf[:n])
	if err != nil {
		return 0, fmt.Errorf(
			"chunk store: failed reading location: %v for chunk %s from sharky: %w",
			rIdx.Location, rIdx.Address, err,
		)
	}
	return n, nil
}

// LocationToChunkLocation converts internal sharky.Location into storage.ChunkLocation.
func LocationToChunkLocation(l sharky.Location) storage.ChunkLocation {
	var cl storage.ChunkLocation
	cl[0] = l.Shard
	binary.BigEndian.PutUint32(cl[1:5], l.Slot)
	binary.BigEndian.PutUint16(cl[5:7], l.Length)
	return cl
}

// ChunkLocationToLocation converts storage.ChunkLocation back into sharky.Location.
func ChunkLocationToLocation(cl storage.ChunkLocation) sharky.Location {
	return sharky.Location{
		Shard:  cl[0],
		Slot:   binary.BigEndian.Uint32(cl[1:5]),
		Length: binary.BigEndian.Uint16(cl[5:7]),
	}
}

// GetIntoLoc reads chunk data using a storage.ChunkLocation hint.
// If the location is zero or was freed during an active sampling session, or if
// reading sharky directly fails, it falls back to standard index-based GetInto.
func GetIntoLoc(
	ctx context.Context,
	r storage.Reader,
	s storage.Sharky,
	guard *LocationGuard,
	addr swarm.Address,
	loc storage.ChunkLocation,
	buf []byte,
) (int, error) {
	if loc.IsZero() || guard == nil || !guard.SessionActive() || guard.IsFreed(loc) {
		return GetInto(ctx, r, s, addr, buf)
	}

	shLoc := ChunkLocationToLocation(loc)
	n := int(shLoc.Length)
	if len(buf) < n {
		return 0, fmt.Errorf("chunk store: buffer too small: %d < %d", len(buf), n)
	}

	err := s.Read(ctx, shLoc, buf[:n])
	if err != nil {
		// Fallback to verified lookup in case of error
		return GetInto(ctx, r, s, addr, buf)
	}
	return n, nil
}

// helper to read chunk from retrievalIndex.
func readChunk(ctx context.Context, s storage.Sharky, rIdx *RetrievalIndexItem) (swarm.Chunk, error) {
	buf := make([]byte, rIdx.Location.Length)
	err := s.Read(ctx, rIdx.Location, buf)
	if err != nil {
		return nil, fmt.Errorf(
			"chunk store: failed reading location: %v for chunk %s from sharky: %w",
			rIdx.Location, rIdx.Address, err,
		)
	}

	return swarm.NewChunk(rIdx.Address, buf), nil
}

func Has(_ context.Context, r storage.Reader, addr swarm.Address) (bool, error) {
	return r.Has(&RetrievalIndexItem{Address: addr})
}

func Put(ctx context.Context, s storage.IndexStore, sh storage.Sharky, ch swarm.Chunk) error {
	_, err := PutLoc(ctx, s, sh, ch)
	return err
}

func PutLoc(ctx context.Context, s storage.IndexStore, sh storage.Sharky, ch swarm.Chunk) (storage.ChunkLocation, error) {
	var (
		rIdx = &RetrievalIndexItem{Address: ch.Address()}
		loc  sharky.Location
	)
	err := s.Get(rIdx)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		// if this is the first instance of this address, we should store the chunk
		// in sharky and create the new indexes.
		loc, err = sh.Write(ctx, ch.Data())
		if err != nil {
			return storage.ChunkLocation{}, fmt.Errorf("chunk store: write to sharky failed: %w", err)
		}
		rIdx.Location = loc
		rIdx.Timestamp = uint64(time.Now().Unix())
	case err != nil:
		return storage.ChunkLocation{}, fmt.Errorf("chunk store: failed to read: %w", err)
	default:
		loc = rIdx.Location
	}

	rIdx.RefCnt++

	return LocationToChunkLocation(loc), s.Put(rIdx)
}

func Replace(ctx context.Context, s storage.IndexStore, sh storage.Sharky, guard *LocationGuard, ch swarm.Chunk, emplace bool) error {
	_, err := ReplaceLoc(ctx, s, sh, guard, ch, emplace)
	return err
}

func ReplaceLoc(ctx context.Context, s storage.IndexStore, sh storage.Sharky, guard *LocationGuard, ch swarm.Chunk, emplace bool) (storage.ChunkLocation, error) {
	rIdx := &RetrievalIndexItem{Address: ch.Address()}
	err := s.Get(rIdx)
	if err != nil {
		return storage.ChunkLocation{}, fmt.Errorf("chunk store: failed to read retrievalIndex for address %s: %w", ch.Address(), err)
	}

	oldLoc := rIdx.Location
	if guard != nil {
		guard.MarkFreed(LocationToChunkLocation(oldLoc))
	}

	err = sh.Release(ctx, oldLoc)
	if err != nil {
		return storage.ChunkLocation{}, fmt.Errorf("chunkstore: failed to release sharky location: %w", err)
	}

	loc, err := sh.Write(ctx, ch.Data())
	if err != nil {
		return storage.ChunkLocation{}, fmt.Errorf("chunk store: write to sharky failed: %w", err)
	}
	rIdx.Location = loc
	rIdx.Timestamp = uint64(time.Now().Unix())
	if emplace {
		rIdx.RefCnt++
	}
	return LocationToChunkLocation(loc), s.Put(rIdx)
}

func Delete(ctx context.Context, s storage.IndexStore, sh storage.Sharky, guard *LocationGuard, addr swarm.Address) error {
	rIdx := &RetrievalIndexItem{Address: addr}
	err := s.Get(rIdx)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("chunk store: failed to read retrievalIndex for address %s: %w", addr, err)
	default:
		rIdx.RefCnt--
	}

	if rIdx.RefCnt > 0 { // If there are more references for this we don't delete it from sharky.
		err = s.Put(rIdx)
		if err != nil {
			return fmt.Errorf("chunk store: failed updating retrievalIndex for address %s: %w", addr, err)
		}
		return nil
	}

	if guard != nil {
		guard.MarkFreed(LocationToChunkLocation(rIdx.Location))
	}

	return errors.Join(
		sh.Release(ctx, rIdx.Location),
		s.Delete(rIdx),
	)
}

func Iterate(ctx context.Context, s storage.IndexStore, sh storage.Sharky, fn storage.IterateChunkFn) error {
	return s.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(RetrievalIndexItem) },
		},
		func(r storage.Result) (bool, error) {
			ch, err := readChunk(ctx, sh, r.Entry.(*RetrievalIndexItem))
			if err != nil {
				return true, err
			}
			return fn(ch)
		},
	)
}

func IterateChunkEntries(st storage.Reader, fn func(swarm.Address, uint32) (bool, error)) error {
	return st.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(RetrievalIndexItem) },
		},
		func(r storage.Result) (bool, error) {
			item := r.Entry.(*RetrievalIndexItem)
			addr := item.Address
			return fn(addr, item.RefCnt)
		},
	)
}

type LocationResult struct {
	Err      error
	Location sharky.Location
}

type IterateResult struct {
	Err  error
	Item *RetrievalIndexItem
}

// IterateLocations iterates over entire retrieval index and plucks only sharky location.
func IterateLocations(
	ctx context.Context,
	st storage.Reader,
) <-chan LocationResult {
	locationResultC := make(chan LocationResult)

	go func() {
		defer close(locationResultC)

		err := st.Iterate(storage.Query{
			Factory: func() storage.Item { return new(RetrievalIndexItem) },
		}, func(r storage.Result) (bool, error) {
			entry := r.Entry.(*RetrievalIndexItem)
			result := LocationResult{Location: entry.Location}

			select {
			case <-ctx.Done():
				return true, ctx.Err()
			case locationResultC <- result:
			}

			return false, nil
		})
		if err != nil {
			result := LocationResult{Err: fmt.Errorf("iterate retrieval index error: %w", err)}

			select {
			case <-ctx.Done():
			case locationResultC <- result:
			}
		}
	}()

	return locationResultC
}

// Iterate iterates over entire retrieval index with a call back.
func IterateItems(st storage.Store, callBackFunc func(*RetrievalIndexItem) error) error {
	return st.Iterate(storage.Query{
		Factory: func() storage.Item { return new(RetrievalIndexItem) },
	}, func(r storage.Result) (bool, error) {
		entry := r.Entry.(*RetrievalIndexItem)
		return false, callBackFunc(entry)
	})
}

// RetrievalIndexItem is the index which gives us the sharky location from the swarm.Address.
// The RefCnt stores the reference of each time a Put operation is issued on this Address.
type RetrievalIndexItem struct {
	Address   swarm.Address
	Timestamp uint64
	Location  sharky.Location
	RefCnt    uint32
}

func (r *RetrievalIndexItem) ID() string { return r.Address.ByteString() }

func (RetrievalIndexItem) Namespace() string { return "retrievalIdx" }

// Stored in bytes as:
// |--Address(32)--|--Timestamp(8)--|--Location(7)--|--RefCnt(4)--|
func (r *RetrievalIndexItem) Marshal() ([]byte, error) {
	if r.Address.IsZero() {
		return nil, errMarshalInvalidRetrievalIndexAddress
	}

	buf := make([]byte, RetrievalIndexItemSize)
	i := 0

	locBuf, err := r.Location.MarshalBinary()
	if err != nil {
		return nil, errMarshalInvalidRetrievalIndexLocation
	}

	copy(buf[i:swarm.HashSize], r.Address.Bytes())
	i += swarm.HashSize

	binary.LittleEndian.PutUint64(buf[i:i+8], r.Timestamp)
	i += 8

	copy(buf[i:i+sharky.LocationSize], locBuf)
	i += sharky.LocationSize

	binary.LittleEndian.PutUint32(buf[i:], r.RefCnt)

	return buf, nil
}

func (r *RetrievalIndexItem) Unmarshal(buf []byte) error {
	if len(buf) != RetrievalIndexItemSize {
		return errUnmarshalInvalidRetrievalIndexSize
	}

	i := 0
	ni := new(RetrievalIndexItem)

	ni.Address = swarm.NewAddress(slices.Clone(buf[i : i+swarm.HashSize]))
	i += swarm.HashSize

	ni.Timestamp = binary.LittleEndian.Uint64(buf[i : i+8])
	i += 8

	loc := new(sharky.Location)
	if err := loc.UnmarshalBinary(buf[i : i+sharky.LocationSize]); err != nil {
		return errUnmarshalInvalidRetrievalIndexLocationBytes
	}
	ni.Location = *loc
	i += sharky.LocationSize

	ni.RefCnt = binary.LittleEndian.Uint32(buf[i:])

	*r = *ni
	return nil
}

func (r *RetrievalIndexItem) Clone() storage.Item {
	if r == nil {
		return nil
	}
	return &RetrievalIndexItem{
		Address:   r.Address.Clone(),
		Timestamp: r.Timestamp,
		Location:  r.Location,
		RefCnt:    r.RefCnt,
	}
}

func (r RetrievalIndexItem) String() string {
	return storageutil.JoinFields(r.Namespace(), r.ID())
}
