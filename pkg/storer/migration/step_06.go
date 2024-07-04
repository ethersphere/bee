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
			err := addStampHashToBatchRadiusAndChunkBinItems(st)
			if err != nil {
				return fmt.Errorf("batch radius and chunk bin item migration: %w", err)
			}
			logger.Info("done migrating batch radius and chunk bin items")
			err = addStampHashToStampIndexItems(st)
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

func addStampHashToBatchRadiusAndChunkBinItems(st transaction.Storage) error {
	itemC := make(chan *OldBatchRadiusItem)
	errC := make(chan error)

	go func() {
		for oldBatchRadiusItem := range itemC {
			err := st.Run(context.Background(), func(s transaction.Store) error {
				idxStore := s.IndexStore()
				stamp, err := chunkstamp.LoadWithBatchID(idxStore, "reserve", oldBatchRadiusItem.Address, oldBatchRadiusItem.BatchID)
				if err != nil {
					return err
				}
				stampHash, err := stamp.Hash()
				if err != nil {
					return err
				}

				// Since the ID format has changed, we should delete the old item and put a new one with the new ID format.
				err = idxStore.Delete(oldBatchRadiusItem)
				if err != nil {
					return err
				}
				err = idxStore.Put(&reserve.BatchRadiusItem{
					Bin:       oldBatchRadiusItem.Bin,
					BatchID:   oldBatchRadiusItem.BatchID,
					StampHash: stampHash,
					Address:   oldBatchRadiusItem.Address,
					BinID:     oldBatchRadiusItem.BinID,
				})
				if err != nil {
					return err
				}

				oldChunkBinItem := &OldChunkBinItem{&reserve.ChunkBinItem{
					Bin:   oldBatchRadiusItem.Bin,
					BinID: oldBatchRadiusItem.BinID,
				}}
				err = idxStore.Get(oldChunkBinItem)
				if err != nil {
					return err
				}

				// same id. Will replace.
				return idxStore.Put(&reserve.ChunkBinItem{
					Bin:       oldChunkBinItem.Bin,
					BinID:     oldChunkBinItem.BinID,
					Address:   oldChunkBinItem.Address,
					BatchID:   oldChunkBinItem.BatchID,
					StampHash: stampHash,
					ChunkType: oldChunkBinItem.ChunkType,
				})
			})
			if err != nil {
				errC <- err
				return
			}
		}
		close(errC)
	}()

	err := st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &OldBatchRadiusItem{&reserve.BatchRadiusItem{}} },
	}, func(result storage.Result) (bool, error) {
		item := result.Entry.(*OldBatchRadiusItem)
		select {
		case itemC <- item:
		case err := <-errC:
			return true, err
		}
		return false, nil
	})
	close(itemC)
	if err != nil {
		return err
	}

	return <-errC
}

func addStampHashToStampIndexItems(st transaction.Storage) error {
	itemC := make(chan *OldStampIndexItem)
	errC := make(chan error)

	go func() {
		for oldStampIndexItem := range itemC {
			err := st.Run(context.Background(), func(s transaction.Store) error {
				idxStore := s.IndexStore()
				stamp, err := chunkstamp.LoadWithBatchID(idxStore, string(oldStampIndexItem.GetNamespace()), oldStampIndexItem.ChunkAddress, oldStampIndexItem.BatchID)
				if err != nil {
					return err
				}
				stampHash, err := stamp.Hash()
				if err != nil {
					return err
				}

				// same id. Will replace.
				item := &stampindex.Item{
					BatchID:          oldStampIndexItem.BatchID,
					StampIndex:       oldStampIndexItem.StampIndex,
					StampHash:        stampHash,
					StampTimestamp:   oldStampIndexItem.StampIndex,
					ChunkAddress:     oldStampIndexItem.ChunkAddress,
					ChunkIsImmutable: oldStampIndexItem.ChunkIsImmutable,
				}
				item.SetNamespace(oldStampIndexItem.GetNamespace())
				return idxStore.Put(item)
			})
			if err != nil {
				errC <- err
				return
			}
		}
		close(errC)
	}()

	err := st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &OldStampIndexItem{&stampindex.Item{}} },
	}, func(result storage.Result) (bool, error) {
		item := result.Entry.(*OldStampIndexItem)
		select {
		case itemC <- item:
		case err := <-errC:
			return true, err
		}
		return false, nil
	})
	close(itemC)
	if err != nil {
		return err
	}

	return <-errC
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
