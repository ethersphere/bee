// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// step_06 is a migration step that adds a stampHash to all BatchRadiusItems, ChunkBinItems and StampIndexItems.
func step_06(st transaction.Storage) func() error {
	return func() error {
		logger := log.NewLogger("migration-step-06", log.WithSink(os.Stdout))
		logger.Info("start adding stampHash to BatchRadiusItems, ChunkBinItems and StampIndexItems")
		err := st.Run(context.Background(), func(s transaction.Store) error {
			err := addStampHash(s.IndexStore(), &OldBatchRadiusItem{&reserve.BatchRadiusItem{}})
			if err != nil {
				return fmt.Errorf("batch radius migration: %w", err)
			}
			logger.Info("done migrating batch radius items")
			err = addStampHash(s.IndexStore(), &OldChunkBinItem{&reserve.ChunkBinItem{}})
			if err != nil {
				return fmt.Errorf("chunk bin migration: %w", err)
			}
			logger.Info("done migrating chunk bin items")
			err = addStampHash(s.IndexStore(), &OldStampIndexItem{&stampindex.Item{}})
			if err != nil {
				return fmt.Errorf("stamp index migration: %w", err)
			}
			logger.Info("done migrating stamp index items")
			return nil
		})
		if err != nil {
			return err
		}
		logger.Info("finished migrating items")
		return nil
	}
}

// addStampHash adds a stampHash to a storage item.
// only BatchRadiusItem and ChunkBinItem are supported.
func addStampHash(st storage.IndexStore, fact storage.Item) error {
	return st.Iterate(storage.Query{
		Factory: func() storage.Item { return fact },
	}, func(res storage.Result) (bool, error) {
		var (
			addr    swarm.Address
			batchID []byte
		)

		switch t := res.Entry.(type) {
		case *OldChunkBinItem:
			item := res.Entry.(*OldChunkBinItem)
			addr = item.Address
			batchID = item.BatchID
		case *OldBatchRadiusItem:
			item := res.Entry.(*OldBatchRadiusItem)
			addr = item.Address
			batchID = item.BatchID
		case *OldStampIndexItem:
			item := res.Entry.(*OldStampIndexItem)
			addr = item.ChunkAddress
			batchID = item.BatchID
		default:
			return true, fmt.Errorf("unsupported item type: %T", t)
		}

		stamp, err := chunkstamp.LoadWithBatchID(st, "reserve", addr, batchID)
		if err != nil {
			return true, fmt.Errorf("load chunkstamp: %w", err)
		}
		hash, err := stamp.Hash()
		if err != nil {
			return true, fmt.Errorf("hash stamp: %w", err)
		}

		switch res.Entry.(type) {
		case *OldChunkBinItem:
			item := res.Entry.(*OldChunkBinItem)
			newItem := &reserve.ChunkBinItem{
				Bin:       item.Bin,
				BinID:     item.BinID,
				Address:   item.Address,
				BatchID:   item.BatchID,
				StampHash: hash,
				ChunkType: item.ChunkType,
			}
			err = st.Put(newItem) // replaces old item with same id.
		case *OldBatchRadiusItem:
			item := res.Entry.(*OldBatchRadiusItem)

			// Since the ID format has changed, we should delete the old item and put a new one with the new ID format.
			err = st.Delete(item)
			if err != nil {
				return true, fmt.Errorf("delete old batch radius item: %w", err)
			}
			newItem := &reserve.BatchRadiusItem{
				Bin:       item.Bin,
				BatchID:   item.BatchID,
				StampHash: hash,
				Address:   item.Address,
				BinID:     item.BinID,
			}
			err = st.Put(newItem)
		case *OldStampIndexItem:
			item := res.Entry.(*OldStampIndexItem)
			newItem := &stampindex.Item{
				BatchID:          item.BatchID,
				StampIndex:       item.StampIndex,
				StampHash:        hash,
				StampTimestamp:   item.StampIndex,
				ChunkAddress:     item.ChunkAddress,
				ChunkIsImmutable: item.ChunkIsImmutable,
			}
			newItem.SetNamespace(item.GetNamespace())
			err = st.Put(newItem) // replaces old item with same id.
		}
		if err != nil {
			return true, fmt.Errorf("put item: %w", err)
		}
		return false, nil
	})
}

type OldChunkBinItem struct {
	*reserve.ChunkBinItem
}

// Unmarshal unmarshals the old chunk bin item that does not include a stamp hash.
func (c *OldChunkBinItem) Unmarshal(buf []byte) error {
	i := 0
	c.Bin = buf[i]
	i += 1

	c.BinID = binary.BigEndian.Uint64(buf[i : i+8])
	i += 8

	c.Address = swarm.NewAddress(buf[i : i+swarm.HashSize]).Clone()
	i += swarm.HashSize

	c.BatchID = copyBytes(buf[i : i+swarm.HashSize])
	i += swarm.HashSize

	c.ChunkType = swarm.ChunkType(buf[i])
	c.StampHash = swarm.EmptyAddress.Bytes()

	return nil
}

type OldBatchRadiusItem struct {
	*reserve.BatchRadiusItem
}

// ID returns the old ID format for BatchRadiusItem ID. (batchId/bin/ChunkAddr).
func (b *OldBatchRadiusItem) ID() string {
	return string(b.BatchID) + string(b.Bin) + b.Address.ByteString()
}

// Unmarshal unmarshals the old batch radius item that does not include a stamp hash.
func (b *OldBatchRadiusItem) Unmarshal(buf []byte) error {
	i := 0
	b.Bin = buf[i]
	i += 1

	b.BatchID = copyBytes(buf[i : i+swarm.HashSize])
	i += swarm.HashSize

	b.Address = swarm.NewAddress(buf[i : i+swarm.HashSize]).Clone()
	i += swarm.HashSize

	b.BinID = binary.BigEndian.Uint64(buf[i : i+8])
	b.StampHash = swarm.EmptyAddress.Bytes()
	return nil
}

type OldStampIndexItem struct {
	*stampindex.Item
}

// Unmarshal unmarhsals the old stamp index item that does not include a stamp hash.
func (i *OldStampIndexItem) Unmarshal(bytes []byte) error {
	nsLen := int(binary.LittleEndian.Uint64(bytes))
	ni := &OldStampIndexItem{&stampindex.Item{}}
	l := 8
	namespace := append(make([]byte, 0, nsLen), bytes[l:l+nsLen]...)
	l += nsLen
	ni.BatchID = append(make([]byte, 0, swarm.HashSize), bytes[l:l+swarm.HashSize]...)
	l += swarm.HashSize
	ni.StampIndex = append(make([]byte, 0, swarm.StampIndexSize), bytes[l:l+swarm.StampIndexSize]...)
	l += swarm.StampIndexSize
	ni.StampTimestamp = append(make([]byte, 0, swarm.StampTimestampSize), bytes[l:l+swarm.StampTimestampSize]...)
	l += swarm.StampTimestampSize
	ni.ChunkAddress = internal.AddressOrZero(bytes[l : l+swarm.HashSize])
	l += swarm.HashSize
	switch bytes[l] {
	case '0':
		ni.ChunkIsImmutable = false
	case '1':
		ni.ChunkIsImmutable = true
	default:
		return errors.New("immutable invalid")
	}
	ni.StampHash = swarm.EmptyAddress.Bytes()
	*i = *ni
	i.SetNamespace(namespace)
	return nil
}

func copyBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
