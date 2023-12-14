// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/encryption"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/storer/internal"
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

// creates a new UUID and returns it as a byte slice
func newUUID() []byte {
	id := uuid.New()
	return id[:]
}

// emptyKey is a 32 byte slice of zeros used to check if encryption key is set
var emptyKey = make([]byte, 32)

// CollectionStat is used to store some basic stats about the pinning collection
type CollectionStat struct {
	Total           uint64
	DupInCollection uint64
}

// pinCollectionSize represents the size of the pinCollectionItem
const pinCollectionItemSize = encryption.ReferenceSize + uuidSize + 8 + 8

var _ storage.Item = (*pinCollectionItem)(nil)

// pinCollectionItem is the index used to describe a pinning collection. The Addr
// is the root reference of the collection and UUID is a unique UUID for this collection.
// The Address could be an encrypted swarm hash. This hash has the key to decrypt the
// collection.
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
	copy(buf[:encryption.ReferenceSize], p.Addr.Bytes())
	off := encryption.ReferenceSize
	copy(buf[off:off+uuidSize], p.UUID)
	statBufOff := encryption.ReferenceSize + uuidSize
	binary.LittleEndian.PutUint64(buf[statBufOff:], p.Stat.Total)
	binary.LittleEndian.PutUint64(buf[statBufOff+8:], p.Stat.DupInCollection)
	return buf, nil
}

func (p *pinCollectionItem) Unmarshal(buf []byte) error {
	if len(buf) != pinCollectionItemSize {
		return errInvalidPinCollectionSize
	}
	ni := new(pinCollectionItem)
	if bytes.Equal(buf[swarm.HashSize:encryption.ReferenceSize], emptyKey) {
		ni.Addr = swarm.NewAddress(buf[:swarm.HashSize]).Clone()
	} else {
		ni.Addr = swarm.NewAddress(buf[:encryption.ReferenceSize]).Clone()
	}
	off := encryption.ReferenceSize
	ni.UUID = append(make([]byte, 0, uuidSize), buf[off:off+uuidSize]...)
	statBuf := buf[off+uuidSize:]
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

func (p pinCollectionItem) String() string {
	return storageutil.JoinFields(p.Namespace(), p.ID())
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

func (p pinChunkItem) String() string {
	return storageutil.JoinFields(p.Namespace(), p.ID())
}

type dirtyCollection struct {
	UUID []byte
}

func (d *dirtyCollection) ID() string { return string(d.UUID) }

func (dirtyCollection) Namespace() string { return "dirtyCollection" }

func (d *dirtyCollection) Marshal() ([]byte, error) {
	return nil, nil
}

func (d *dirtyCollection) Unmarshal(_ []byte) error {
	return nil
}

func (d *dirtyCollection) Clone() storage.Item {
	if d == nil {
		return nil
	}
	return &dirtyCollection{
		UUID: append([]byte(nil), d.UUID...),
	}
}

func (d dirtyCollection) String() string {
	return storageutil.JoinFields(d.Namespace(), d.ID())
}

// NewCollection returns a putter wrapped around the passed storage.
// The putter will add the chunk to Chunk store if it doesnt exists within this collection.
// It will create a new UUID for the collection which can be used to iterate on all the chunks
// that are part of this collection. The root pin is only updated on successful close of this
// Putter.
func NewCollection(st internal.Storage) (internal.PutterCloserWithReference, error) {
	newCollectionUUID := newUUID()
	err := st.IndexStore().Put(&dirtyCollection{UUID: newCollectionUUID})
	if err != nil {
		return nil, err
	}
	return &collectionPutter{
		collection: &pinCollectionItem{UUID: newCollectionUUID},
	}, nil
}

type collectionPutter struct {
	mtx        sync.Mutex
	collection *pinCollectionItem
	closed     bool
}

func (c *collectionPutter) Put(ctx context.Context, st internal.Storage, writer storage.Writer, ch swarm.Chunk) error {
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
	found, err := st.IndexStore().Has(collectionChunk)
	if err != nil {
		return fmt.Errorf("pin store: failed to check chunk: %w", err)
	}
	if found {
		// If we already have this chunk in the current collection, don't add it
		// again.
		c.collection.Stat.DupInCollection++
		return nil
	}

	err = writer.Put(collectionChunk)
	if err != nil {
		return fmt.Errorf("pin store: failed putting collection chunk: %w", err)
	}

	err = st.ChunkStore().Put(ctx, ch)
	if err != nil {
		return fmt.Errorf("pin store: failled putting chunk: %w", err)
	}

	return nil
}

func (c *collectionPutter) Close(st internal.Storage, writer storage.Writer, root swarm.Address) error {
	if root.IsZero() {
		return errCollectionRootAddressIsZero
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Save the root pin reference.
	c.collection.Addr = root
	err := writer.Put(c.collection)
	if err != nil {
		return fmt.Errorf("pin store: failed updating collection: %w", err)
	}

	err = writer.Delete(&dirtyCollection{UUID: c.collection.UUID})
	if err != nil {
		return fmt.Errorf("pin store: failed deleting dirty collection: %w", err)
	}

	c.closed = true
	return nil
}

func (c *collectionPutter) Cleanup(tx internal.TxExecutor) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.closed {
		return nil
	}

	if err := deleteCollectionChunks(context.Background(), tx, c.collection.UUID); err != nil {
		return fmt.Errorf("pin store: failed deleting collection chunks: %w", err)
	}

	err := tx.Execute(context.Background(), func(s internal.Storage) error {
		return s.IndexStore().Delete(&dirtyCollection{UUID: c.collection.UUID})
	})
	if err != nil {
		return fmt.Errorf("pin store: failed deleting dirty collection: %w", err)
	}

	c.closed = true
	return nil
}

// CleanupDirty will iterate over all the dirty collections and delete them.
func CleanupDirty(tx internal.TxExecutor) error {
	dirtyCollections := make([]*dirtyCollection, 0)
	err := tx.Execute(context.Background(), func(s internal.Storage) error {
		return s.IndexStore().Iterate(
			storage.Query{
				Factory:      func() storage.Item { return new(dirtyCollection) },
				ItemProperty: storage.QueryItemID,
			},
			func(r storage.Result) (bool, error) {
				di := &dirtyCollection{UUID: []byte(r.ID)}
				dirtyCollections = append(dirtyCollections, di)
				return false, nil
			},
		)
	})
	if err != nil {
		return fmt.Errorf("pin store: failed iterating dirty collections: %w", err)
	}

	for _, di := range dirtyCollections {
		_ = (&collectionPutter{collection: &pinCollectionItem{UUID: di.UUID}}).Cleanup(tx)
	}

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
func Pins(st storage.Store, offset, limit int) ([]swarm.Address, error) {
	var pins []swarm.Address
	err := st.Iterate(storage.Query{
		Factory:      func() storage.Item { return new(pinCollectionItem) },
		ItemProperty: storage.QueryItemID,
	}, func(r storage.Result) (bool, error) {
		if offset > 0 {
			offset--
			return false, nil
		}
		addr := swarm.NewAddress([]byte(r.ID))
		pins = append(pins, addr)
		if limit > 0 {
			limit--
			if limit == 0 {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("pin store: failed iterating root refs: %w", err)
	}

	return pins, nil
}

func deleteCollectionChunks(ctx context.Context, tx internal.TxExecutor, collectionUUID []byte) error {
	chunksToDelete := make([]*pinChunkItem, 0)
	err := tx.Execute(ctx, func(s internal.Storage) error {
		return s.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return &pinChunkItem{UUID: collectionUUID} },
			}, func(r storage.Result) (bool, error) {
				addr := swarm.NewAddress([]byte(r.ID))
				chunk := &pinChunkItem{UUID: collectionUUID, Addr: addr}
				chunksToDelete = append(chunksToDelete, chunk)
				return false, nil
			},
		)
	})
	if err != nil {
		return fmt.Errorf("pin store: failed iterating collection chunks: %w", err)
	}

	batchCnt := 1000
	for i := 0; i < len(chunksToDelete); i += batchCnt {
		err = tx.Execute(context.Background(), func(s internal.Storage) error {
			b, err := s.IndexStore().Batch(context.Background())
			if err != nil {
				return err
			}

			end := i + batchCnt
			if end > len(chunksToDelete) {
				end = len(chunksToDelete)
			}

			for _, chunk := range chunksToDelete[i:end] {
				err := b.Delete(chunk)
				if err != nil {
					return fmt.Errorf("pin store: failed deleting collection chunk: %w", err)
				}
				err = s.ChunkStore().Delete(ctx, chunk.Addr)
				if err != nil {
					return fmt.Errorf("pin store: failed in tx chunk deletion: %w", err)
				}
			}
			return b.Commit()
		})
		if err != nil {
			return fmt.Errorf("pin store: failed tx deleting collection chunks: %w", err)
		}
	}
	return nil
}

// DeletePin will delete the root pin and all the chunks that are part of this
// collection.
func DeletePin(ctx context.Context, tx internal.TxExecutor, root swarm.Address) error {
	collection := &pinCollectionItem{Addr: root}
	err := tx.Execute(context.Background(), func(s internal.Storage) error {
		return s.IndexStore().Get(collection)
	})
	if err != nil {
		return fmt.Errorf("pin store: failed getting collection: %w", err)
	}

	if err := deleteCollectionChunks(ctx, tx, collection.UUID); err != nil {
		return err
	}

	err = tx.Execute(context.Background(), func(s internal.Storage) error {
		return s.IndexStore().Delete(collection)
	})
	if err != nil {
		return fmt.Errorf("pin store: failed deleting root collection: %w", err)
	}

	return nil
}

func IterateCollection(st storage.Store, root swarm.Address, fn func(addr swarm.Address) (bool, error)) error {
	collection := &pinCollectionItem{Addr: root}
	err := st.Get(collection)
	if err != nil {
		return fmt.Errorf("pin store: failed getting collection: %w", err)
	}

	return st.Iterate(storage.Query{
		Factory:      func() storage.Item { return &pinChunkItem{UUID: collection.UUID} },
		ItemProperty: storage.QueryItemID,
	}, func(r storage.Result) (bool, error) {
		addr := swarm.NewAddress([]byte(r.ID))
		stop, err := fn(addr)
		if err != nil {
			return true, err
		}
		return stop, nil
	})
}

func IterateCollectionStats(st storage.Store, iterateFn func(st CollectionStat) (bool, error)) error {
	return st.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(pinCollectionItem) },
		},
		func(r storage.Result) (bool, error) {
			return iterateFn(r.Entry.(*pinCollectionItem).Stat)
		},
	)
}
