// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"encoding/binary"
	"encoding/hex"
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
	"golang.org/x/exp/slices"
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

func (c *ChunkStoreWrapper) GetRefCnt(ctx context.Context, addr swarm.Address) (uint32, error) {
	rIdx := &RetrievalIndexItem{Address: addr}
	err := c.store.Get(rIdx)
	if err != nil {
		return 0, fmt.Errorf("chunk store: failed reading retrievalIndex for address %s: %w", addr, err)
	}
	return rIdx.RefCnt, nil
}

func (c *ChunkStoreWrapper) Has(_ context.Context, addr swarm.Address) (bool, error) {
	return c.store.Has(&RetrievalIndexItem{Address: addr})
}

func (c *ChunkStoreWrapper) Put(ctx context.Context, ch swarm.Chunk, why string) error {
	var (
		rIdx  = &RetrievalIndexItem{Address: ch.Address()}
		loc   sharky.Location
		found = true
		stamp = ch.Stamp()
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
		if stamp == nil {
		c.logger.Debug("chunkTrace: chunkStore.Put new chunk NOstamp!", "why", why, "address", ch.Address(), "loc", loc.ToString())
		} else {
		c.logger.Debug("chunkTrace: chunkStore.Put new chunk", "why", why, "address", ch.Address(), "loc", loc.ToString(),
						"batch_id", hex.EncodeToString(ch.Stamp().BatchID()),
						"index", hex.EncodeToString(ch.Stamp().Index()))
		}
	case err != nil:
		return fmt.Errorf("chunk store: failed to read: %w", err)
	case err == nil:
		if stamp == nil {
		c.logger.Debug("chunkTrace: chunkStore.Put increment chunk NOstamp!", "why", why, "address", ch.Address(), "loc", rIdx.Location.ToString(), "refCnt", rIdx.RefCnt+1)
		} else {
		c.logger.Debug("chunkTrace: chunkStore.Put increment chunk", "why", why, "address", ch.Address(), "loc", rIdx.Location.ToString(), "refCnt", rIdx.RefCnt+1,
						"batch_id", hex.EncodeToString(ch.Stamp().BatchID()),
						"index", hex.EncodeToString(ch.Stamp().Index()))
		}
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

func (c *ChunkStoreWrapper) Delete(ctx context.Context, addr swarm.Address, why string) error {
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
		c.logger.Debug("chunkTrace: chunkStore.Delete decrement chunk", "why", why, "address", addr, "loc", rIdx.Location.ToString(), "refCnt", rIdx.RefCnt)
		err = c.store.Put(rIdx)
		if err != nil {
			return fmt.Errorf("chunk store: failed updating retrievalIndex for address %s: %w", addr, err)
		}
		return nil
	}
	c.logger.Debug("chunkTrace: chunkStore.Delete delete chunk", "why", why, "address", addr, "loc", rIdx.Location.ToString(), "refCnt", rIdx.RefCnt)

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
