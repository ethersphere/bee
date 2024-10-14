// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"golang.org/x/sync/errgroup"
)

const reserveScope = "reserve"

type baseAddress struct {
	sync.Mutex
	b []byte
}

func (a *baseAddress) Clone() *baseAddress {
	a.Lock()
	defer a.Lock()

	if a.b == nil {
		return &baseAddress{}
	}
	copied := make([]byte, len(a.b))
	copy(copied, a.b)
	return &baseAddress{b: copied}
}

type AtomicByteArray struct {
	data unsafe.Pointer
}

func NewAtomicByteArray(initialData []byte) *AtomicByteArray {
	return &AtomicByteArray{
		data: unsafe.Pointer(&initialData),
	}
}

// Load atomically loads and returns a copy of the byte array
func (a *AtomicByteArray) Bytes() []byte {
	dataPtr := (*[]byte)(atomic.LoadPointer(&a.data))
	if dataPtr == nil {
		return nil
	}
	copyData := make([]byte, len(*dataPtr))
	copy(copyData, *dataPtr)
	return copyData
}

// Bytes returns bytes representation of the Address.
func (a *baseAddress) Bytes() []byte {
	a.Lock()
	defer a.Lock()
	copied := make([]byte, len(a.b))
	copy(copied, a.b)
	return copied
}

type Reserve struct {
	putBaseAddr *baseAddress
	hasBaseAddr *baseAddress
	getBaseAddr *baseAddress

	radiusSetter topology.SetStorageRadiuser
	logger       log.Logger

	capacity int
	size     atomic.Int64
	radius   atomic.Uint32

	multx *swarm.MultexLock
	st    transaction.Storage
	// mut   sync.Mutex
}

type Address struct {
	sync.Mutex
	b []byte
}

func (a *Address) Clone() Address {
	a.Lock()
	defer a.Unlock()

	if a.b == nil {
		return Address{}
	}
	copied := make([]byte, len(a.b))
	copy(copied, a.b)
	return Address{b: copied}
}

func (a *Address) Bytes() []byte {
	a.Lock()
	defer a.Unlock()
	copied := make([]byte, len(a.b))
	copy(copied, a.b)
	return copied
}

var address Address

func New(
	baseAddr swarm.Address,
	st transaction.Storage,
	capacity int,
	radiusSetter topology.SetStorageRadiuser,
	logger log.Logger,
) (*Reserve, error) {

	address = Address{b: baseAddr.Clone().Bytes()}

	rs := &Reserve{
		putBaseAddr:  &baseAddress{b: baseAddr.Clone().Bytes()},
		getBaseAddr:  &baseAddress{b: baseAddr.Clone().Bytes()},
		hasBaseAddr:  &baseAddress{b: baseAddr.Clone().Bytes()},
		st:           st,
		capacity:     capacity,
		radiusSetter: radiusSetter,
		logger:       logger.WithName(reserveScope).Register(),
		multx:        swarm.NewMultexLock(),
	}

	err := st.Run(context.Background(), func(s transaction.Store) error {
		rItem := &radiusItem{}
		err := s.IndexStore().Get(rItem)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		rs.radius.Store(uint32(rItem.Radius))

		epochItem := &EpochItem{}
		err = s.IndexStore().Get(epochItem)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				err := s.IndexStore().Put(&EpochItem{Timestamp: uint64(time.Now().Unix())})
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		size, err := s.IndexStore().Count(&BatchRadiusItem{})
		if err != nil {
			return err
		}
		rs.size.Store(int64(size))
		return nil
	})

	return rs, err
}

// Reserve Put has to handle multiple possible scenarios.
//  1. Since the same chunk may belong to different postage batches, the reserve will support one chunk to many postage
//     batches relationship.
//  2. A new chunk that shares the same stamp index belonging to the same batch with an already stored chunk will overwrite
//     the existing chunk if the new chunk has a higher stamp timestamp (regardless of batch type).
//  3. A new chunk that has the same address belonging to the same batch with an already stored chunk will overwrite the existing chunk
//     if the new chunk has a higher stamp timestamp (regardless of batch type and chunk type, eg CAC & SOC).
func (r *Reserve) Put(ctx context.Context, chunk swarm.Chunk) error {
	fmt.Printf("hello put4444444444\n")
	chunkType := storage.ChunkType(chunk)
	chunkAddress := chunk.Address().Clone()

	fmt.Printf("before bucket calc , after chunkaddress \n")
	bucket := postage.ToBucket(postage.BucketDepth, chunkAddress)
	lockId := LockKey(bucket)
	reserveLocker.Lock(lockId)
	defer reserveLocker.Unlock(lockId)

	fmt.Printf("after chunkaddress clone\n")

	fmt.Printf("before baseaddr\n")
	baseAddr := address.Bytes()
	chunkAddrBytes := chunk.Address().Clone().Bytes()
	fmt.Printf("between baseaddr\n")
	fmt.Printf("after baseaddr\n")
	bin := swarm.Proximity(baseAddr, chunkAddress.Bytes())
	fmt.Printf("after Proximity\n")

	// bin lock

	// r.multx.Lock(strconv.Itoa(int(bin)))
	// defer r.multx.Unlock(strconv.Itoa(int(bin)))

	fmt.Printf("Reserve Put chunk stamp available: %x\n", chunk.Stamp().Index())

	stampHash, err := chunk.Stamp().Hash()
	if err != nil {
		return err
	}

	batchId := chunk.Stamp().BatchID()[:]
	fmt.Printf("after batchid %x\n", batchId)
	// batchID lock, Put vs Eviction
	// fmt.Printf("before batchid lock\n")
	// r.multx.Lock(string(batchId))
	// defer r.multx.Unlock(string(batchId))
	// lockId2 := "putlock" + string(batchId)
	// reserveLocker.Lock(lockId2)
	// defer reserveLocker.Unlock(lockId2)

	// check if the chunk with the same batch, stamp timestamp and index is already stored
	item := &BatchRadiusItem{Bin: swarm.Proximity(baseAddr, chunkAddrBytes), BatchID: batchId, Address: chunkAddress, StampHash: stampHash}
	fmt.Printf("Before has\n")
	indexStore := r.IndexStore()
	fmt.Printf("Got indexstore\n")
	has, err := indexStore.Has(item)
	fmt.Printf("after has, which is %v and error %s\n", has, err)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	fmt.Printf("After has\n")

	var shouldIncReserveSize, shouldDecrReserveSize bool

	err = r.st.Run(ctx, func(s transaction.Store) error {
		fmt.Printf("+-------------- Reserve Put chunk stamp available: %x\n", chunk.Stamp().Index())
		oldStampIndex, loadedStampIndex, err := stampindex.LoadOrStore(s.IndexStore(), reserveScope, chunk)
		if err != nil {
			return fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
		}
		fmt.Printf("After load or store\n")

		sameAddressOldStamp, err := chunkstamp.LoadWithBatchID(s.IndexStore(), reserveScope, chunkAddress, chunk.Stamp().BatchID())
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		fmt.Printf("Hello0: %x\n", chunk.Stamp().Index())

		// same chunk address, same batch
		if sameAddressOldStamp != nil {
			sameAddressOldStampIndex, err := stampindex.Load(s.IndexStore(), reserveScope, sameAddressOldStamp)
			if err != nil {
				return err
			}
			fmt.Printf("Hello1: %x\n", chunk.Stamp().Index())
			defer func() { fmt.Printf("Hello2: %x\n", chunk.Stamp().Index()) }()

			// same index
			if bytes.Equal(sameAddressOldStamp.Index(), chunk.Stamp().Index()) {
				prev := binary.BigEndian.Uint64(sameAddressOldStampIndex.StampTimestamp)
				curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
				if prev >= curr {
					return fmt.Errorf("overwrite same chunk. prev %d cur %d batch %s: %w", prev, curr, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
				}

				oldBatchRadiusItem := &BatchRadiusItem{
					Bin:       bin,
					Address:   chunkAddress,
					BatchID:   sameAddressOldStampIndex.BatchID,
					StampHash: sameAddressOldStampIndex.StampHash,
				}
				// load item to get the binID
				err = s.IndexStore().Get(oldBatchRadiusItem)
				if err != nil {
					return err
				}

				// delete old chunk index items
				err = errors.Join(
					s.IndexStore().Delete(oldBatchRadiusItem),
					s.IndexStore().Delete(&ChunkBinItem{Bin: oldBatchRadiusItem.Bin, BinID: oldBatchRadiusItem.BinID}),
					stampindex.Delete(s.IndexStore(), reserveScope, sameAddressOldStamp),
					chunkstamp.DeleteWithStamp(s.IndexStore(), reserveScope, oldBatchRadiusItem.Address, sameAddressOldStamp),
				)
				if err != nil {
					return err
				}

				binID, err := r.IncBinID(s.IndexStore(), bin)
				if err != nil {
					return err
				}

				err = errors.Join(
					stampindex.Store(s.IndexStore(), reserveScope, chunk),
					chunkstamp.Store(s.IndexStore(), reserveScope, chunk),
					s.IndexStore().Put(&BatchRadiusItem{
						Bin:       bin,
						BinID:     binID,
						Address:   chunkAddress,
						BatchID:   batchId,
						StampHash: stampHash,
					}),
					s.IndexStore().Put(&ChunkBinItem{
						Bin:       bin,
						BinID:     binID,
						Address:   chunkAddress,
						BatchID:   batchId,
						ChunkType: chunkType,
						StampHash: stampHash,
					}),
				)
				if err != nil {
					return err
				}

				if chunkType != swarm.ChunkTypeSingleOwner {
					return nil
				}

				r.logger.Debug("replacing soc in chunkstore", "address", chunkAddress)
				return s.ChunkStore().Replace(ctx, chunk)
			}
		}

		// different address, same batch, index collision
		if loadedStampIndex && !chunkAddress.Equal(oldStampIndex.ChunkAddress) {
			prev := binary.BigEndian.Uint64(oldStampIndex.StampTimestamp)
			curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
			if prev >= curr {
				return fmt.Errorf("overwrite prev %d cur %d batch %s: %w", prev, curr, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
			}
			// An older (same or different) chunk with the same batchID and stamp index has been previously
			// saved to the reserve. We must do the below before saving the new chunk:
			// 1. Delete the old chunk from the chunkstore.
			// 2. Delete the old chunk's stamp data.
			// 3. Delete ALL old chunk related items from the reserve.
			// 4. Update the stamp index.

			err = r.removeChunk(ctx, s, oldStampIndex.ChunkAddress, oldStampIndex.BatchID, oldStampIndex.StampHash, baseAddr)
			if err != nil {
				return fmt.Errorf("failed removing older chunk %s: %w", oldStampIndex.ChunkAddress, err)
			}

			r.logger.Warning(
				"replacing chunk stamp index",
				"old_chunk", oldStampIndex.ChunkAddress,
				"new_chunk", chunkAddress,
				"batch_id", hex.EncodeToString(batchId),
			)

			// replace old stamp index.
			err = stampindex.Store(s.IndexStore(), reserveScope, chunk)
			if err != nil {
				return fmt.Errorf("failed updating stamp index: %w", err)
			}
		}

		binID, err := r.IncBinID(s.IndexStore(), bin)
		if err != nil {
			return err
		}

		err = errors.Join(
			chunkstamp.Store(s.IndexStore(), reserveScope, chunk),
			s.IndexStore().Put(&BatchRadiusItem{
				Bin:       bin,
				BinID:     binID,
				Address:   chunkAddress,
				BatchID:   batchId,
				StampHash: stampHash,
			}),
			s.IndexStore().Put(&ChunkBinItem{
				Bin:       bin,
				BinID:     binID,
				Address:   chunkAddress,
				BatchID:   batchId,
				ChunkType: chunkType,
				StampHash: stampHash,
			}),
			s.ChunkStore().Put(ctx, chunk),
		)
		if err != nil {
			return err
		}

		if !loadedStampIndex {
			shouldIncReserveSize = true
		}

		return nil
	})
	if err != nil {
		return err
	}
	if shouldIncReserveSize {
		r.size.Add(1)
	}
	if shouldDecrReserveSize {
		r.size.Add(-1)
	}
	return nil
}

func (r *Reserve) Has(addr swarm.Address, batchID []byte, stampHash []byte) (bool, error) {
	// r.mu <- struct{}{}
	// lockId2 := "putlock" + string(batchID)
	// reserveLocker.Lock(lockId2)
	// defer reserveLocker.Unlock(lockId2)

	// baseAddr := r.hasBaseAddr.Bytes()
	fmt.Printf("before baseaddr reserve has\n")
	baseAddr := address.Bytes()
	// <-r.mu
	item := &BatchRadiusItem{Bin: swarm.Proximity(baseAddr, addr.Bytes()), BatchID: batchID, Address: addr, StampHash: stampHash}
	return r.IndexStore().Has(item)
}

func (r *Reserve) Get(ctx context.Context, addr swarm.Address, batchID []byte, stampHash []byte) (swarm.Chunk, error) {
	// lockId2 := "putlock" + string(batchID)
	// reserveLocker.Lock(lockId2)
	// defer reserveLocker.Unlock(lockId2)

	// r.mu <- struct{}{}
	baseAddr := r.getBaseAddr.Bytes()
	// <-r.mu
	indexStore := r.IndexStore()
	item := &BatchRadiusItem{Bin: swarm.Proximity(baseAddr, addr.Bytes()), BatchID: batchID, Address: addr, StampHash: stampHash}
	err := indexStore.Get(item)
	if err != nil {
		return nil, err
	}

	stamp, err := chunkstamp.LoadWithBatchID(indexStore, reserveScope, addr, item.BatchID)
	if err != nil {
		return nil, err
	}

	ch, err := r.st.ChunkStore().Get(ctx, addr)
	if err != nil {
		return nil, err
	}

	return ch.WithStamp(stamp), nil
}

// EvictBatchBin evicts all chunks from bins upto the bin provided.
func (r *Reserve) EvictBatchBin(
	ctx context.Context,
	batchID []byte,
	count int,
	bin uint8,
) (int, error) {

	r.multx.Lock(string(batchID))
	defer r.multx.Unlock(string(batchID))

	var evicteditems []*BatchRadiusItem

	if count <= 0 {
		return 0, nil
	}

	err := r.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &BatchRadiusItem{} },
		Prefix:  string(batchID),
	}, func(res storage.Result) (bool, error) {
		batchRadius := res.Entry.(*BatchRadiusItem)
		if batchRadius.Bin >= bin {
			return true, nil
		}
		evicteditems = append(evicteditems, batchRadius)
		count--
		if count == 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return 0, err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(runtime.NumCPU())

	var evicted atomic.Int64

	for _, item := range evicteditems {
		func(item *BatchRadiusItem) {
			eg.Go(func() error {
				err := r.st.Run(ctx, func(s transaction.Store) error {
					return RemoveChunkWithItem(ctx, s, item)
				})
				if err != nil {
					return err
				}
				evicted.Add(1)
				return nil
			})
		}(item)
	}

	err = eg.Wait()

	r.size.Add(-evicted.Load())

	return int(evicted.Load()), err
}

func (r *Reserve) removeChunk(
	ctx context.Context,
	trx transaction.Store,
	chunkAddress swarm.Address,
	baseAddr []byte,
	batchID []byte,
	stampHash []byte,
) error {
	item := &BatchRadiusItem{
		Bin:       swarm.Proximity(baseAddr, chunkAddress.Bytes()),
		BatchID:   batchID,
		Address:   chunkAddress,
		StampHash: stampHash,
	}
	err := trx.IndexStore().Get(item)
	if err != nil {
		return err
	}
	return RemoveChunkWithItem(ctx, trx, item)
}

func RemoveChunkWithItem(
	ctx context.Context,
	trx transaction.Store,
	item *BatchRadiusItem,
) error {

	var errs error

	stamp, _ := chunkstamp.LoadWithBatchID(trx.IndexStore(), reserveScope, item.Address, item.BatchID)
	if stamp != nil {
		errs = errors.Join(
			stampindex.Delete(trx.IndexStore(), reserveScope, stamp),
			chunkstamp.DeleteWithStamp(trx.IndexStore(), reserveScope, item.Address, stamp),
		)
	}

	return errors.Join(errs,
		trx.IndexStore().Delete(item),
		trx.IndexStore().Delete(&ChunkBinItem{Bin: item.Bin, BinID: item.BinID}),
		trx.ChunkStore().Delete(ctx, item.Address),
	)
}

func (r *Reserve) IterateBin(bin uint8, startBinID uint64, cb func(swarm.Address, uint64, []byte, []byte) (bool, error)) error {
	err := r.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(bin, startBinID),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)
		if item.Bin > bin {
			return true, nil
		}

		stop, err := cb(item.Address, item.BinID, item.BatchID, item.StampHash)
		if stop || err != nil {
			return true, err
		}

		return false, nil
	})

	return err
}

func (r *Reserve) IterateChunks(startBin uint8, cb func(swarm.Chunk) (bool, error)) error {
	indexStore := r.IndexStore()
	err := indexStore.Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(startBin, 0),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)

		chunk, err := r.st.ChunkStore().Get(context.Background(), item.Address)
		if err != nil {
			return false, err
		}

		stamp, err := chunkstamp.LoadWithBatchID(indexStore, reserveScope, item.Address, item.BatchID)
		if err != nil {
			return false, err
		}

		stop, err := cb(chunk.WithStamp(stamp))
		if stop || err != nil {
			return true, err
		}
		return false, nil
	})

	return err
}

func (r *Reserve) IterateChunksItems(startBin uint8, cb func(*ChunkBinItem) (bool, error)) error {
	err := r.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(startBin, 0),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)

		stop, err := cb(item)
		if stop || err != nil {
			return true, err
		}
		return false, nil
	})

	return err
}

// Reset removes all the entires in the reserve. Must be done before any calls to the reserve.
func (r *Reserve) Reset(ctx context.Context) error {

	size := r.Size()

	bRitems := make([]*BatchRadiusItem, 0, size)
	indexStore := r.IndexStore()

	err := indexStore.Iterate(storage.Query{
		Factory: func() storage.Item { return &BatchRadiusItem{} },
	}, func(res storage.Result) (bool, error) {
		bRitems = append(bRitems, res.Entry.(*BatchRadiusItem))
		return false, nil
	})
	if err != nil {
		return err
	}

	var eg errgroup.Group
	eg.SetLimit(runtime.NumCPU())

	for _, item := range bRitems {
		item := item
		eg.Go(func() error {
			return r.st.Run(ctx, func(s transaction.Store) error {
				return errors.Join(
					s.ChunkStore().Delete(ctx, item.Address),
					s.IndexStore().Delete(item),
					s.IndexStore().Delete(&ChunkBinItem{Bin: item.Bin, BinID: item.BinID}),
				)
			})
		})
	}

	err = eg.Wait()
	if err != nil {
		return err
	}
	bRitems = nil

	sitems := make([]*stampindex.Item, 0, size)
	err = indexStore.Iterate(storage.Query{
		Factory: func() storage.Item { return &stampindex.Item{} },
	}, func(res storage.Result) (bool, error) {
		sitems = append(sitems, res.Entry.(*stampindex.Item))
		return false, nil
	})
	if err != nil {
		return err
	}
	for _, item := range sitems {
		item := item
		eg.Go(func() error {
			return r.st.Run(ctx, func(s transaction.Store) error {
				return errors.Join(
					s.IndexStore().Delete(item),
					chunkstamp.DeleteWithStamp(s.IndexStore(), reserveScope, item.ChunkAddress, postage.NewStamp(item.BatchID, item.StampIndex, item.StampTimestamp, nil)),
				)
			})
		})
	}

	err = eg.Wait()
	if err != nil {
		return err
	}
	sitems = nil

	r.size.Store(0)

	return nil
}

func (r *Reserve) Radius() uint8 {
	return uint8(r.radius.Load())
}

func (r *Reserve) Size() int {
	return int(r.size.Load())
}

func (r *Reserve) Capacity() int {
	return r.capacity
}

func (r *Reserve) IsWithinCapacity() bool {
	return int(r.size.Load()) <= r.capacity
}

func (r *Reserve) EvictionTarget() int {
	if r.IsWithinCapacity() {
		return 0
	}
	return int(r.size.Load()) - r.capacity
}

func (r *Reserve) SetRadius(rad uint8) error {
	r.radius.Store(uint32(rad))
	r.radiusSetter.SetStorageRadius(rad)
	return r.st.Run(context.Background(), func(s transaction.Store) error {
		return s.IndexStore().Put(&radiusItem{Radius: rad})
	})
}

func (r *Reserve) LastBinIDs() ([]uint64, uint64, error) {
	var epoch EpochItem
	indexStore := r.IndexStore()
	err := indexStore.Get(&epoch)
	if err != nil {
		return nil, 0, err
	}

	ids := make([]uint64, swarm.MaxBins)

	for bin := uint8(0); bin < swarm.MaxBins; bin++ {
		binItem := &BinItem{Bin: bin}
		err := indexStore.Get(binItem)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				ids[bin] = 0
			} else {
				return nil, 0, err
			}
		} else {
			ids[bin] = binItem.BinID
		}
	}

	return ids, epoch.Timestamp, nil
}

func (r *Reserve) IncBinID(store storage.IndexStore, bin uint8) (uint64, error) {
	item := &BinItem{Bin: bin}
	err := store.Get(item)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			item.BinID = 1
			return 1, store.Put(item)
		}

		return 0, err
	}

	item.BinID += 1

	return item.BinID, store.Put(item)
}

func (r *Reserve) IndexStore() storage.Reader {
	// r.mut.Lock()
	// defer r.mut.Unlock()
	return r.st.IndexStore()
}
