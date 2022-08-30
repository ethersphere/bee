// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/uuid"
)

const (
	hashSize = 32
	uuidSize = 36
)

var batchSize = 100

type CollectionStat struct {
	Total           uint64
	DupInCollection uint64
}

type pinCollection struct {
	Addr swarm.Address
	UUID string
	Stat CollectionStat
}

func (pinCollection) Namespace() string { return "pinCollection" }

func (p *pinCollection) ID() string { return p.Addr.ByteString() }

func (p *pinCollection) Marshal() ([]byte, error) {
	buf := p.Addr.Bytes()
	buf = append(buf, []byte(p.UUID)...)
	statBuf := make([]byte, 8+8)
	binary.LittleEndian.PutUint64(statBuf[:8], p.Stat.Total)
	binary.LittleEndian.PutUint64(statBuf[8:16], p.Stat.DupInCollection)
	buf = append(buf, statBuf...)
	return buf, nil
}

func (p *pinCollection) Unmarshal(buf []byte) error {
	p.Addr = swarm.NewAddress(buf[:hashSize])
	p.UUID = string(buf[hashSize : hashSize+uuidSize])
	statBuf := buf[hashSize+uuidSize:]
	p.Stat.Total = binary.LittleEndian.Uint64(statBuf[:8])
	p.Stat.DupInCollection = binary.LittleEndian.Uint64(statBuf[8:16])
	return nil
}

type pinChunk struct {
	UUID string
	Addr swarm.Address
}

func (p *pinChunk) Namespace() string { return p.UUID }

func (p *pinChunk) ID() string { return p.Addr.ByteString() }

func (p *pinChunk) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (p *pinChunk) Unmarshal(_ []byte) error {
	return nil
}

type PutterCloser interface {
	storage.Putter
	io.Closer
}

// NewCollection returns a putter wrapped around the passed storage for indexes and chunks.
// The putter will add the chunk to Chunk store if it doesnt exists within this collection.
// It will create a new UUID for the collection which can be used to iterate on all the chunks
// that are part of this collection.
func NewCollection(
	st storage.Store,
	chSt storage.ChunkStore,
	root swarm.Address,
) (PutterCloser, error) {

	collection := &pinCollection{Addr: root, UUID: uuid.NewString()}
	found, err := st.Has(collection)
	if err != nil {
		return nil, fmt.Errorf("pin store: failed checking collection: %w", err)
	}
	if found {
		return nil, fmt.Errorf("pin store: root %s already exists", root)
	}

	err = st.Put(collection)
	if err != nil {
		return nil, fmt.Errorf("pin store: failed adding new collection: %w", err)
	}

	return &collectionPutter{collection: collection, st: st, chSt: chSt}, nil
}

type collectionPutter struct {
	mtx        sync.Mutex
	collection *pinCollection
	st         storage.Store
	chSt       storage.ChunkStore
}

func (c *collectionPutter) Put(ctx context.Context, ch swarm.Chunk) (bool, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.collection.Stat.Total++

	collectionChunk := &pinChunk{UUID: c.collection.UUID, Addr: ch.Address()}
	found, err := c.st.Has(collectionChunk)
	if err != nil {
		return false, fmt.Errorf("pin store: failed to check chunk: %w", err)
	}
	if found {
		c.collection.Stat.DupInCollection++
		return true, nil
	}

	err = c.st.Put(collectionChunk)
	if err != nil {
		return false, fmt.Errorf("pin store: failed putting collection chunk: %w", err)
	}

	_, err = c.chSt.Put(ctx, ch)
	if err != nil {
		return false, fmt.Errorf("pin store: failled putting chunk: %w", err)
	}

	return false, nil
}

func (c *collectionPutter) Close() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	err := c.st.Put(c.collection)
	if err != nil {
		return fmt.Errorf("pin store: failed updating collection: %w", err)
	}
	return nil
}

func HasPin(st storage.Store, root swarm.Address) (bool, error) {
	collection := &pinCollection{Addr: root}
	has, err := st.Has(collection)
	if err != nil {
		return false, fmt.Errorf("pin store: failed checking collection: %w", err)
	}
	return has, nil
}

func Pins(st storage.Store) ([]swarm.Address, error) {
	var pins []swarm.Address
	err := st.Iterate(storage.Query{
		Factory:       func() storage.Item { return new(pinCollection) },
		ItemAttribute: storage.QueryItemID,
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

func DeletePin(st storage.Store, chSt storage.ChunkStore, root swarm.Address) error {
	collection := &pinCollection{Addr: root}
	err := st.Get(collection)
	if err != nil {
		return fmt.Errorf("pin store: failed getting collection: %w", err)
	}

	var offset uint64
	total := collection.Stat.Total - collection.Stat.DupInCollection
	for ; offset < total; offset += uint64(batchSize) {
		addrsToDelete := make([]swarm.Address, 0, batchSize)
		countInBatch := 0

		err = st.Iterate(storage.Query{
			Factory:       func() storage.Item { return &pinChunk{UUID: collection.UUID} },
			ItemAttribute: storage.QueryItemID,
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
			chunk := &pinChunk{UUID: collection.UUID, Addr: addr}
			err := st.Delete(chunk)
			if err != nil {
				return fmt.Errorf("pin store: failed in batch deletion: %w", err)
			}
			err = chSt.Delete(context.TODO(), chunk.Addr)
			if err != nil {
				return fmt.Errorf("pin store: failed in batch chunk deletion: %w", err)
			}
		}
	}

	err = chSt.Delete(context.TODO(), collection.Addr)
	if err != nil {
		return fmt.Errorf("pin store: failed deleting root chunk: %w", err)
	}
	err = st.Delete(collection)
	if err != nil {
		return fmt.Errorf("pin store: failed deleting root collection: %w", err)
	}

	return nil
}
