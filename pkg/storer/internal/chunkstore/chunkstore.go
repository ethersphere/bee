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

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

var (
	// errMarshalInvalidRetrievalIndexAddress is returned if the retrievalIndexItem address is zero during marshaling.
	errMarshalInvalidRetrievalIndexAddress = errors.New("marshal retrievalIndexItem: address is zero")
	// errMarshalInvalidRetrievalIndexLocation is returned if the retrievalIndexItem location is invalid during marshaling.
	errMarshalInvalidRetrievalIndexLocation = errors.New("marshal retrievalIndexItem: location is invalid")
	// errUnmarshalInvalidRetrievalIndexSize is returned during unmarshaling if the passed buffer is not the expected size.
	errUnmarshalInvalidRetrievalIndexSize = errors.New("unmarshal retrievalIndexItem: invalid size")
	// errUnmarshalInvalidRetrievalIndexLocationBytes is returned during unmarshaling if the location buffer is invalid.
	errUnmarshalInvalidRetrievalIndexLocationBytes = errors.New("unmarshal retrievalIndexItem: invalid location bytes")
)

const retrievalIndexItemSize = swarm.HashSize + 8 + sharky.LocationSize + 1

var _ storage.Item = (*retrievalIndexItem)(nil)

// retrievalIndexItem is the index which gives us the sharky location from the swarm.Address.
// The RefCnt stores the reference of each time a Put operation is issued on this Address.
type retrievalIndexItem struct {
	Address   swarm.Address
	Timestamp uint64
	Location  sharky.Location
	RefCnt    uint8
}

func (r *retrievalIndexItem) ID() string { return r.Address.ByteString() }

func (retrievalIndexItem) Namespace() string { return "retrievalIdx" }

// Stored in bytes as:
// |--Address(32)--|--Timestamp(8)--|--Location(7)--|--RefCnt(1)--|
func (r *retrievalIndexItem) Marshal() ([]byte, error) {
	if r.Address.IsZero() {
		return nil, errMarshalInvalidRetrievalIndexAddress
	}

	locBuf, err := r.Location.MarshalBinary()
	if err != nil {
		return nil, errMarshalInvalidRetrievalIndexLocation
	}

	buf := make([]byte, retrievalIndexItemSize)
	copy(buf, r.Address.Bytes())
	binary.LittleEndian.PutUint64(buf[swarm.HashSize:], r.Timestamp)
	copy(buf[swarm.HashSize+8:], locBuf)
	buf[retrievalIndexItemSize-1] = r.RefCnt
	return buf, nil
}

func (r *retrievalIndexItem) Unmarshal(buf []byte) error {
	if len(buf) != retrievalIndexItemSize {
		return errUnmarshalInvalidRetrievalIndexSize
	}

	loc := new(sharky.Location)
	if err := loc.UnmarshalBinary(buf[swarm.HashSize+8:]); err != nil {
		return errUnmarshalInvalidRetrievalIndexLocationBytes
	}

	ni := new(retrievalIndexItem)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	ni.Timestamp = binary.LittleEndian.Uint64(buf[swarm.HashSize:])
	ni.Location = *loc
	ni.RefCnt = buf[retrievalIndexItemSize-1]
	*r = *ni
	return nil
}

func (r *retrievalIndexItem) Clone() storage.Item {
	if r == nil {
		return nil
	}
	return &retrievalIndexItem{
		Address:   r.Address.Clone(),
		Timestamp: r.Timestamp,
		Location:  r.Location,
		RefCnt:    r.RefCnt,
	}
}

func (r retrievalIndexItem) String() string {
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

type chunkStoreWrapper struct {
	store  storage.Store
	sharky Sharky
}

// helper to read chunk from retrievalIndex.
func (c *chunkStoreWrapper) readChunk(ctx context.Context, rIdx *retrievalIndexItem) (swarm.Chunk, error) {
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

func (c *chunkStoreWrapper) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	rIdx := &retrievalIndexItem{Address: addr}
	err := c.store.Get(rIdx)
	if err != nil {
		return nil, fmt.Errorf("chunk store: failed reading retrievalIndex for address %s: %w", addr, err)
	}
	return c.readChunk(ctx, rIdx)
}

func (c *chunkStoreWrapper) Has(_ context.Context, addr swarm.Address) (bool, error) {
	return c.store.Has(&retrievalIndexItem{Address: addr})
}

func (c *chunkStoreWrapper) Put(ctx context.Context, ch swarm.Chunk) error {
	var (
		rIdx  = &retrievalIndexItem{Address: ch.Address()}
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
	case err != nil:
		return fmt.Errorf("chunk store: failed to read: %w", err)
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
		return multierror.Append(
			err,
			c.sharky.Release(context.Background(), loc),
		)
	}

	return nil
}

func (c *chunkStoreWrapper) Delete(ctx context.Context, addr swarm.Address) error {
	rIdx := &retrievalIndexItem{Address: addr}
	err := c.store.Get(rIdx)
	if err != nil {
		return fmt.Errorf("chunk store: failed to read retrievalIndex for address %s: %w", addr, err)
	}

	rIdx.RefCnt--
	if rIdx.RefCnt > 0 {
		// if there are more references for this we dont delete it from sharky.
		err = c.store.Put(rIdx)
		if err != nil {
			return fmt.Errorf("chunk store: failed updating retrievalIndex for address %s: %w", addr, err)
		}
		return nil
	}

	// delete the chunk.
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

func (c *chunkStoreWrapper) Iterate(ctx context.Context, fn storage.IterateChunkFn) error {
	return c.store.Iterate(storage.Query{
		Factory: func() storage.Item { return new(retrievalIndexItem) },
	}, func(r storage.Result) (bool, error) {
		ch, err := c.readChunk(ctx, r.Entry.(*retrievalIndexItem))
		if err != nil {
			return true, err
		}
		return fn(ch)
	})
}

func (c *chunkStoreWrapper) Close() error { return nil }
