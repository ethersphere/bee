// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

const (
	retrievalIndexItemSize = swarm.HashSize + 8 + sharky.LocationSize + 1
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

	// errMarshalInvalidChunkStampAddress is returned during marshaling if the Address is zero.
	errMarshalInvalidChunkStampItemAddress = errors.New("marshal chunkStampItem: invalid address")
	// errUnmarshalInvalidChunkStampAddress is returned during unmarshaling if the address is not set.
	errUnmarshalInvalidChunkStampItemAddress = errors.New("unmarshal chunkStampItem: invalid address")
	// errMarshalInvalidChunkStamp is returned if the stamp is invalid during marshaling.
	errMarshalInvalidChunkStampItemStamp = errors.New("marshal chunkStampItem: invalid stamp")
	// errUnmarshalInvalidChunkStampSize is returned during unmarshaling if the passed buffer is not the expected size.
	errUnmarshalInvalidChunkStampItemSize = errors.New("unmarshal chunkStampItem: invalid size")
)

// retrievalIndexItem is the index which gives us the sharky location from the swarm.Address.
// The RefCnt stores the reference of each time a Put operation is issued on this Address.
type retrievalIndexItem struct {
	Address   swarm.Address
	Timestamp uint64
	Location  sharky.Location
	RefCnt    uint8
}

func (retrievalIndexItem) Namespace() string { return "retrievalIdx" }

func (r *retrievalIndexItem) ID() string { return r.Address.ByteString() }

// Stored in bytes as:
// |--Address--|--Timestamp--|--Location--|--RefCnt--|
//      32            8            7           1
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

// chunkStampItem is the index used to represent a stamp for a chunk. Going ahead we will
// support multiple stamps on chunks. This item will allow mapping multiple stamps to a
// single address. For this reason, the Address is part of the Namespace and can be used
// to iterate on all the stamps for this Address.
type chunkStampItem struct {
	Address swarm.Address
	Stamp   swarm.Stamp
}

func (c *chunkStampItem) Namespace() string {
	return path.Join("stamp", c.Address.ByteString())
}

func (c *chunkStampItem) ID() string {
	return path.Join(string(c.Stamp.BatchID()), string(c.Stamp.Index()))
}

// Address is not part of the payload which is stored, as Address is part of the prefix,
// hence already known before querying this object. This will be reused during unmarshaling.
// Stored in bytes as
// |--BatchID--|--Index--|--Timestamp--|--Signature--|
//      32          8           8             65
func (c *chunkStampItem) Marshal() ([]byte, error) {
	// The address is not part of the payload, but it is used to create the namespace
	// so it is better if we check that the address is correctly set here before it
	// is stored in the underlying storage.
	if c.Address.IsZero() {
		return nil, errMarshalInvalidChunkStampItemAddress
	}
	if c.Stamp == nil {
		return nil, errMarshalInvalidChunkStampItemStamp
	}
	buf, err := c.Stamp.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed marshaling stamp: %w", err)
	}
	return buf, nil
}

func (c *chunkStampItem) Unmarshal(buf []byte) error {
	// ensure that the address is set already in the item.
	if c.Address.IsZero() {
		return errUnmarshalInvalidChunkStampItemAddress
	}
	stamp := new(postage.Stamp)
	if err := stamp.UnmarshalBinary(buf); err != nil {
		if errors.Is(err, postage.ErrStampInvalid) {
			return errUnmarshalInvalidChunkStampItemSize
		}
		return fmt.Errorf("failed unmarshaling stamp: %w", err)
	}

	ni := new(chunkStampItem)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), c.Address.Bytes()...))
	ni.Stamp = stamp
	*c = *ni
	return nil
}

type chunkStoreWrapper struct {
	store  storage.Store
	sharky *sharky.Store
}

// New returns a chunkStoreWrapper which implements the storage.ChunkStore interface
// on top of the Store and the sharky blob storage.
// It uses the Store to maintain the retrievalIndex and also stamps for the chunk and
// sharky to maintain the chunk data.
// Currently we do not support multiple stamps on chunks, but the chunkStoreWrapper
// allows for this in future.
// chunkStoreWrapper uses refCnt and works slightly different to the ChunkStore API.
// The current Put API returns true if the chunk is already found as it supports only
// unique chunks. The chunkStoreWrapper will also maintain refCnts for all the Puts
// for the same chunk. This allows duplicates to change state.
// Usually this Put operation would not be used by clients directly as it will be part
// of some component which will provide the uniqueness guarantee.
// Due to the refCnt a Delete would only result in an actual Delete operation
// if the refCnt goes to 0.
func New(store storage.Store, sharky *sharky.Store) storage.ChunkStore {
	return &chunkStoreWrapper{store: store, sharky: sharky}
}

func (c *chunkStoreWrapper) getStamp(addr swarm.Address) (swarm.Stamp, error) {
	var stamp swarm.Stamp
	err := c.store.Iterate(storage.Query{
		Factory: func() storage.Item { return &chunkStampItem{Address: addr} },
	}, func(res storage.Result) (bool, error) {
		// Use the first available stamp till we support multiple stamps on chunk.
		stamp = res.Entry.(*chunkStampItem).Stamp
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return stamp, nil
}

func (c *chunkStoreWrapper) removeStamps(addr swarm.Address) error {
	var stamps []swarm.Stamp
	err := c.store.Iterate(storage.Query{
		Factory: func() storage.Item { return &chunkStampItem{Address: addr} },
	}, func(res storage.Result) (bool, error) {
		stamps = append(stamps, res.Entry.(*chunkStampItem).Stamp)
		return false, nil
	})
	if err != nil {
		return err
	}
	var delErr error
	for _, s := range stamps {
		err := c.store.Delete(&chunkStampItem{Address: addr, Stamp: s})
		if err != nil {
			delErr = multierror.Append(delErr, err)
		}
	}
	return delErr
}

// helper to read chunk from retrievalIndex.
func (c *chunkStoreWrapper) readChunk(ctx context.Context, rIdx *retrievalIndexItem) (swarm.Chunk, error) {
	stamp, err := c.getStamp(rIdx.Address)
	if err != nil {
		return nil, fmt.Errorf("chunk store: failed to read stamp for address %s: %w", rIdx.Address, err)
	}
	if stamp == nil {
		return nil, fmt.Errorf("chunk store: no stamp for chunk %s", rIdx.Address)
	}

	buf := make([]byte, rIdx.Location.Length)
	err = c.sharky.Read(ctx, rIdx.Location, buf)
	if err != nil {
		return nil, fmt.Errorf(
			"chunk store: failed reading location: %v for chunk %s from sharky: %w",
			rIdx.Location, rIdx.Address, err,
		)
	}

	return swarm.NewChunk(rIdx.Address, buf).WithStamp(stamp), nil
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

func (c *chunkStoreWrapper) Put(ctx context.Context, ch swarm.Chunk) (bool, error) {
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
			return false, fmt.Errorf("chunk store: write to sharky failed: %w", err)
		}
		rIdx.Location = loc
		rIdx.Timestamp = uint64(time.Now().Unix())
		found = false
	case err != nil:
		return false, fmt.Errorf("chunk store: failed to read: %w", err)
	}

	rIdx.RefCnt++

	err = func() error {
		err = c.store.Put(rIdx)
		if err != nil {
			return fmt.Errorf("chunk store: failed to update retrievalIndex: %w", err)
		}

		err = c.store.Put(&chunkStampItem{Address: ch.Address(), Stamp: ch.Stamp()})
		if err != nil {
			return fmt.Errorf("chunk store: failed to update chunk stamp: %w", err)
		}
		return nil
	}()

	if err != nil && !found {
		return false, multierror.Append(
			err,
			c.sharky.Release(context.Background(), loc),
		)
	}

	return found, nil
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

	err = c.removeStamps(rIdx.Address)
	if err != nil {
		return fmt.Errorf("chunk store: failed removing stamps for address %s: %w", addr, err)
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
