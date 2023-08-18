// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/soc"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/swarm"
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

const RetrievalIndexItemSize = swarm.HashSize + 8 + sharky.LocationSize + 1

var _ storage.Item = (*RetrievalIndexItem)(nil)

// RetrievalIndexItem is the index which gives us the sharky location from the swarm.Address.
// The RefCnt stores the reference of each time a Put operation is issued on this Address.
type RetrievalIndexItem struct {
	Address   swarm.Address
	Timestamp uint64
	Location  sharky.Location
	RefCnt    uint8
}

func (r *RetrievalIndexItem) ID() string { return r.Address.ByteString() }

func (RetrievalIndexItem) Namespace() string { return "retrievalIdx" }

// Stored in bytes as:
// |--Address(32)--|--Timestamp(8)--|--Location(7)--|--RefCnt(1)--|
func (r *RetrievalIndexItem) Marshal() ([]byte, error) {
	if r.Address.IsZero() {
		return nil, errMarshalInvalidRetrievalIndexAddress
	}

	locBuf, err := r.Location.MarshalBinary()
	if err != nil {
		return nil, errMarshalInvalidRetrievalIndexLocation
	}

	buf := make([]byte, RetrievalIndexItemSize)
	copy(buf, r.Address.Bytes())
	binary.LittleEndian.PutUint64(buf[swarm.HashSize:], r.Timestamp)
	copy(buf[swarm.HashSize+8:], locBuf)
	buf[RetrievalIndexItemSize-1] = r.RefCnt
	return buf, nil
}

func (r *RetrievalIndexItem) Unmarshal(buf []byte) error {
	if len(buf) != RetrievalIndexItemSize {
		return errUnmarshalInvalidRetrievalIndexSize
	}

	loc := new(sharky.Location)
	if err := loc.UnmarshalBinary(buf[swarm.HashSize+8:]); err != nil {
		return errUnmarshalInvalidRetrievalIndexLocationBytes
	}

	ni := new(RetrievalIndexItem)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	ni.Timestamp = binary.LittleEndian.Uint64(buf[swarm.HashSize:])
	ni.Location = *loc
	ni.RefCnt = buf[RetrievalIndexItemSize-1]
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

// Sharky provides an abstraction for the sharky.Store operations used in the
// chunkstore. This allows us to be more flexible in passing in the sharky instance
// to chunkstore. For eg, check the TxChunkStore implementation in this pkg.
type Sharky interface {
	Read(context.Context, sharky.Location, []byte) error
	Write(context.Context, []byte) (sharky.Location, error)
	Release(context.Context, sharky.Location) error
}

type ChunkStoreWrapper struct {
	store  storage.Store
	sharky Sharky
	logger log.Logger
}

func New(store storage.Store, sharky Sharky, logger log.Logger) *ChunkStoreWrapper {
	return &ChunkStoreWrapper{store: store, sharky: sharky, logger: logger}
}

// helper to read chunk from retrievalIndex.
func (c *ChunkStoreWrapper) readChunk(ctx context.Context, rIdx *RetrievalIndexItem) (swarm.Chunk, error) {
	buf := make([]byte, rIdx.Location.Length)
	err := c.sharky.Read(ctx, rIdx.Location, buf)
	if err != nil {
		return nil, fmt.Errorf(
			"chunk store: failed reading location: %v for chunk %s from sharky: %w",
			rIdx.Location, rIdx.Address, err,
		)
	}

	return swarm.NewChunk(rIdx.Address, buf), nil
}

func (c *ChunkStoreWrapper) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	rIdx := &RetrievalIndexItem{Address: addr}
	err := c.store.Get(rIdx)
	if err != nil {
		return nil, fmt.Errorf("chunk store: failed reading retrievalIndex for address %s: %w", addr, err)
	}
	chunk, err := c.readChunk(ctx, rIdx)
	if err != nil {
		return nil, err
	}
	err = cac.Valid(chunk)
	if err != nil && !soc.Valid(chunk) {
//type RetrievalIndexItem struct {
//	Address   swarm.Address
//	Timestamp uint64
//	Location  sharky.Location
//	RefCnt    uint8
//}
//type Location struct {
//	Shard  uint8
//	Slot   uint32
//	Length uint16
//}
		c.logger.Debug("chunkTrace: chunkStore.Get invalid chunk", "address", addr, "err", err)
		return nil, fmt.Errorf("chunk store: read invalid chunk at address %s loc %s ref %d: %w",
								addr, rIdx.Location.ToString(), rIdx.RefCnt, err)
	}
	return chunk, nil
}

func (c *ChunkStoreWrapper) Has(_ context.Context, addr swarm.Address) (bool, error) {
	return c.store.Has(&RetrievalIndexItem{Address: addr})
}

func (c *ChunkStoreWrapper) Put(ctx context.Context, ch swarm.Chunk) error {
	var (
		rIdx  = &RetrievalIndexItem{Address: ch.Address()}
		loc   sharky.Location
		found = true
	)
	err := c.store.Get(rIdx)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		// if this is the first instance of this address, we should store the chunk
		// in sharky and create the new indexes.
		loc, err = c.sharky.Write(ctx, ch.Data())
		if err != nil {
			return fmt.Errorf("chunk store: write to sharky failed: %w", err)
		}
		rIdx.Location = loc
		rIdx.Timestamp = uint64(time.Now().Unix())
		found = false
		c.logger.Debug("chunkTrace: chunkStore.Put new chunk", "address", ch.Address(), "loc", loc.ToString())
	case err != nil:
		return fmt.Errorf("chunk store: failed to read: %w", err)
	case err == nil:
		c.logger.Debug("chunkTrace: chunkStore.Put increment chunk", "address", ch.Address(), "loc", rIdx.Location.ToString(), "refCnt", rIdx.RefCnt+1)
	}

	rIdx.RefCnt++

	err = func() error {
		err = c.store.Put(rIdx)
		if err != nil {
			return fmt.Errorf("chunk store: failed to update retrievalIndex: %w", err)
		}
		return nil
	}()

	if err != nil && !found {
		return errors.Join(
			err,
			c.sharky.Release(context.Background(), loc),
		)
	}

	return nil
}

func (c *ChunkStoreWrapper) Delete(ctx context.Context, addr swarm.Address) error {
	rIdx := &RetrievalIndexItem{Address: addr}
	err := c.store.Get(rIdx)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("chunk store: failed to read retrievalIndex for address %s: %w", addr, err)
	default:
		rIdx.RefCnt--
	}

	if rIdx.RefCnt > 0 { // If there are more references for this we don't delete it from sharky.
		c.logger.Debug("chunkTrace: chunkStore.Delete decrement chunk", "address", addr, "loc", rIdx.Location.ToString(), "refCnt", rIdx.RefCnt)
		err = c.store.Put(rIdx)
		if err != nil {
			return fmt.Errorf("chunk store: failed updating retrievalIndex for address %s: %w", addr, err)
		}
		return nil
	}
	c.logger.Debug("chunkTrace: chunkStore.Delete delete chunk", "address", addr, "loc", rIdx.Location.ToString(), "refCnt", rIdx.RefCnt)

	// Delete the chunk.
	err = c.sharky.Release(ctx, rIdx.Location)
	if err != nil {
		return fmt.Errorf(
			"chunk store: failed to release sharky slot %v for address %s: %w",
			rIdx.Location, rIdx.Address, err,
		)
	}
	err = c.store.Delete(rIdx)
	if err != nil {
		return fmt.Errorf("chunk store: failed to delete retrievalIndex for address %s: %w", addr, err)
	}

	return nil
}

func (c *ChunkStoreWrapper) Iterate(ctx context.Context, fn storage.IterateChunkFn) error {
	return c.store.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(RetrievalIndexItem) },
		},
		func(r storage.Result) (bool, error) {
			ch, err := c.readChunk(ctx, r.Entry.(*RetrievalIndexItem))
			if err != nil {
				return true, err
			}
			return fn(ch)
		},
	)
}

func (c *ChunkStoreWrapper) Close() error {
	return c.store.Close()
}

func IterateChunkEntries(st storage.Store, fn func(swarm.Address, bool) (bool, error)) error {
	return st.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(RetrievalIndexItem) },
		},
		func(r storage.Result) (bool, error) {
			addr := r.Entry.(*RetrievalIndexItem).Address
			isShared := r.Entry.(*RetrievalIndexItem).RefCnt > 1
			return fn(addr, isShared)
		},
	)
}
