// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/uuid"
)

const (
	// size of the UUID generated by the pinstore
	uuidSize = 16
)

var (
	// errInvalidPinCollectionAddr is returned when trying to marshal a pinCollectionItem
	// with a zero address
	errInvalidPinCollectionAddr = errors.New("marshal pinCollectionItem: address is zero")
	// errInvalidPinCollectionUUID is returned when trying to marshal a pinCollectionItem
	// with an empty UUID
	errInvalidPinCollectionUUID = errors.New("marshal pinCollectionItem: UUID is zero")
	// errInvalidPinCollectionSize is returned when trying to unmarshal a buffer of
	// incorrect size
	errInvalidPinCollectionSize = errors.New("unmarshal pinCollectionItem: invalid size")
	// errPutterAlreadyClosed is returned when trying to use a Putter which is already closed
	errPutterAlreadyClosed = errors.New("pin store: putter already closed")
	// errCollectionRootAddressIsZero is returned if the putter is closed with a zero
	// swarm.Address. Root reference has to be set.
	errCollectionRootAddressIsZero = errors.New("pin store: collection root address is zero")
)

// batchSize used for deletion
var batchSize = 100

// creates a new UUID and returns it as a byte slice
func newUUID() []byte {
	id := uuid.New()
	return id[:]
}

// CollectionStat is used to store some basic stats about the pinning collection
type CollectionStat struct {
	Total           uint64
	DupInCollection uint64
}

// pinCollectionSize represents the size of the pinCollectionItem
const pinCollectionItemSize = swarm.HashSize + uuidSize + 8 + 8

var _ storage.Item = (*pinCollectionItem)(nil)

// pinCollectionItem is the index used to describe a pinning collection. The Addr
// is the root reference of the collection and UUID is a unique UUID for this collection.
type pinCollectionItem struct {
	Addr swarm.Address
	UUID []byte
	Stat CollectionStat
}

func (p *pinCollectionItem) ID() string { return p.Addr.ByteString() }

func (pinCollectionItem) Namespace() string { return "pinCollectionItem" }

func (p *pinCollectionItem) Marshal() ([]byte, error) {
	if p.Addr.IsZero() {
		return nil, errInvalidPinCollectionAddr
	}
	if len(p.UUID) == 0 {
		return nil, errInvalidPinCollectionUUID
	}
	buf := make([]byte, pinCollectionItemSize)
	copy(buf[:swarm.HashSize], p.Addr.Bytes())
	copy(buf[swarm.HashSize:swarm.HashSize+uuidSize], p.UUID)
	statBufOff := swarm.HashSize + uuidSize
	binary.LittleEndian.PutUint64(buf[statBufOff:], p.Stat.Total)
	binary.LittleEndian.PutUint64(buf[statBufOff+8:], p.Stat.DupInCollection)
	return buf, nil
}

func (p *pinCollectionItem) Unmarshal(buf []byte) error {
	if len(buf) != pinCollectionItemSize {
		return errInvalidPinCollectionSize
	}
	ni := new(pinCollectionItem)
	ni.Addr = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	ni.UUID = append(make([]byte, 0, uuidSize), buf[swarm.HashSize:swarm.HashSize+uuidSize]...)
	statBuf := buf[swarm.HashSize+uuidSize:]
	ni.Stat.Total = binary.LittleEndian.Uint64(statBuf[:8])
	ni.Stat.DupInCollection = binary.LittleEndian.Uint64(statBuf[8:16])
	*p = *ni
	return nil
}

func (p *pinCollectionItem) Clone() storage.Item {
	if p == nil {
		return nil
	}
	return &pinCollectionItem{
		Addr: p.Addr.Clone(),
		UUID: append([]byte(nil), p.UUID...),
		Stat: p.Stat,
	}
}

var _ storage.Item = (*pinChunkItem)(nil)

// pinChunkItem is the index used to represent a single chunk in the pinning
// collection. It is prefixed with the UUID of the collection.
type pinChunkItem struct {
	UUID []byte
	Addr swarm.Address
}

func (p *pinChunkItem) Namespace() string { return string(p.UUID) }

func (p *pinChunkItem) ID() string { return p.Addr.ByteString() }

// pinChunkItem is a key-only type index. We don't need to store any value. As such
// the serialization functions would be no-ops. A Get operation on this key is not
// required as the key would constitute the item. Usually these type of indexes are
// useful for key-only iterations.
func (p *pinChunkItem) Marshal() ([]byte, error) {
	return nil, nil
}

func (p *pinChunkItem) Unmarshal(_ []byte) error {
	return nil
}

func (p *pinChunkItem) Clone() storage.Item {
	if p == nil {
		return nil
	}
	return &pinChunkItem{
		UUID: append([]byte(nil), p.UUID...),
		Addr: p.Addr.Clone(),
	}
}

// NewCollection returns a putter wrapped around the passed storage.
// The putter will add the chunk to Chunk store if it doesnt exists within this collection.
// It will create a new UUID for the collection which can be used to iterate on all the chunks
// that are part of this collection. The root pin is only updated on successful close of this
// Putter.
func NewCollection(st internal.Storage) internal.PutterCloserWithReference {
	return &collectionPutter{
		collection: &pinCollectionItem{UUID: newUUID()},
		st:         st,
	}
}

type collectionPutter struct {
	mtx        sync.Mutex
	collection *pinCollectionItem
	st         internal.Storage
	closed     bool
}

func (c *collectionPutter) Put(ctx context.Context, ch swarm.Chunk) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// do not allow any Puts after putter was closed
	if c.closed {
		return errPutterAlreadyClosed
	}

	c.collection.Stat.Total++

	// We will only care about duplicates within this collection. In order to
	// guarantee that we dont accidentally delete common chunks across collections,
	// a separate pinCollectionItem entry will be present for each duplicate chunk.
	collectionChunk := &pinChunkItem{UUID: c.collection.UUID, Addr: ch.Address()}
	found, err := c.st.IndexStore().Has(collectionChunk)
	if err != nil {
		return fmt.Errorf("pin store: failed to check chunk: %w", err)
	}
	if found {
		// If we already have this chunk in the current collection, don't add it
		// again.
		c.collection.Stat.DupInCollection++
		return nil
	}

	err = c.st.IndexStore().Put(collectionChunk)
	if err != nil {
		return fmt.Errorf("pin store: failed putting collection chunk: %w", err)
	}

	err = c.st.ChunkStore().Put(ctx, ch)
	if err != nil {
		return fmt.Errorf("pin store: failled putting chunk: %w", err)
	}

	return nil
}

func (c *collectionPutter) Close(root swarm.Address) error {
	if root.IsZero() {
		return errCollectionRootAddressIsZero
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Save the root pin reference.
	c.collection.Addr = root
	err := c.st.IndexStore().Put(c.collection)
	if err != nil {
		return fmt.Errorf("pin store: failed updating collection: %w", err)
	}

	c.closed = true
	return nil
}

// HasPin function will check if the address represents a valid pin collection.
func HasPin(st storage.Store, root swarm.Address) (bool, error) {
	collection := &pinCollectionItem{Addr: root}
	has, err := st.Has(collection)
	if err != nil {
		return false, fmt.Errorf("pin store: failed checking collection: %w", err)
	}
	return has, nil
}

// Pins lists all the added pinning collections.
func Pins(st storage.Store) ([]swarm.Address, error) {
	var pins []swarm.Address
	err := st.Iterate(storage.Query{
		Factory:      func() storage.Item { return new(pinCollectionItem) },
		ItemProperty: storage.QueryItemID,
	}, func(r storage.Result) (bool, error) {
		addr := swarm.NewAddress([]byte(r.ID))
		pins = append(pins, addr)
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("pin store: failed iterating root refs: %w", err)
	}

	return pins, nil
}

// DeletePin will delete the root pin and all the chunks that are part of this
// collection.
func DeletePin(ctx context.Context, st internal.Storage, root swarm.Address) error {
	collection := &pinCollectionItem{Addr: root}
	err := st.IndexStore().Get(collection)
	if err != nil {
		return fmt.Errorf("pin store: failed getting collection: %w", err)
	}

	var offset uint64
	total := collection.Stat.Total - collection.Stat.DupInCollection
	for ; offset < total; offset += uint64(batchSize) {
		addrsToDelete := make([]swarm.Address, 0, batchSize)
		countInBatch := 0

		err = st.IndexStore().Iterate(storage.Query{
			Factory:      func() storage.Item { return &pinChunkItem{UUID: collection.UUID} },
			ItemProperty: storage.QueryItemID,
		}, func(r storage.Result) (bool, error) {
			addr := swarm.NewAddress([]byte(r.ID))
			addrsToDelete = append(addrsToDelete, addr)
			countInBatch++
			if countInBatch == batchSize {
				return true, nil
			}
			return false, nil

		})
		if err != nil {
			return fmt.Errorf("pin store: failed in iteration: %w", err)
		}

		for _, addr := range addrsToDelete {
			chunk := &pinChunkItem{UUID: collection.UUID, Addr: addr}
			err := st.IndexStore().Delete(chunk)
			if err != nil {
				return fmt.Errorf("pin store: failed in batch deletion: %w", err)
			}
			err = st.ChunkStore().Delete(ctx, chunk.Addr)
			if err != nil {
				return fmt.Errorf("pin store: failed in batch chunk deletion: %w", err)
			}
		}
	}

	err = st.IndexStore().Delete(collection)
	if err != nil {
		return fmt.Errorf("pin store: failed deleting root collection: %w", err)
	}

	return nil
}