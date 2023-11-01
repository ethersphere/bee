// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sampler

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash"
	"math/big"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
	storer "github.com/ethersphere/bee/pkg/storer"

	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

const Size = 16

type Batchstore interface {
	postage.ChainStateGetter
	postage.ValueIterator
}

type Sampler struct {
	backend        postage.ChainBackend
	batchstore     Batchstore
	sampler        storer.Sampler
	fullSyncedFunc func() bool
	healthyFunc    func() bool
}

func New(
	backend postage.ChainBackend,
	batchstore Batchstore,
	sampler storer.Sampler,
	fullSyncedFunc,
	healthyFunc func() bool,
) *Sampler {
	return &Sampler{
		batchstore:     batchstore,
		sampler:        sampler,
		fullSyncedFunc: fullSyncedFunc,
		healthyFunc:    healthyFunc,
	}
}

type Sample struct {
	Stats Stats
	Data  Data
}
type Item struct {
	Address      swarm.Address
	TransAddress swarm.Address
	Prover       bmt.Prover
	TransProver  bmt.Prover
	LoadStamp    func() (*postage.Stamp, error)
	Stamp        *postage.Stamp
	SOC          *soc.SOC
}

func newStamp(s swarm.Stamp) *postage.Stamp {
	return postage.NewStamp(s.BatchID(), s.Index(), s.Timestamp(), s.Sig())
}

type Data struct {
	Depth  uint8
	Hash   swarm.Address
	Prover bmt.Prover
	Items  []Item
}

func Chunk(depth uint8, items []Item) (*Data, error) {
	content := make([]byte, len(items)*2*swarm.HashSize)
	for i, s := range items {
		copy(content[i*2*swarm.HashSize:], s.Address.Bytes())
		copy(content[(i*2+1)*swarm.HashSize:], s.TransAddress.Bytes())
	}
	hasher := bmtpool.Get()
	prover := bmt.Prover{Hasher: hasher}
	prover.SetHeaderInt64(int64(len(content)))
	_, err := prover.Write(content)
	if err != nil {
		return nil, err
	}
	hash, err := prover.Hash(nil)
	if err != nil {
		return nil, err
	}
	return &Data{
		Depth:  depth,
		Hash:   swarm.NewAddress(hash),
		Prover: prover,
		Items:  items,
	}, nil
}

func (s *Sampler) MakeSample(ctx context.Context, salt []byte, depth uint8, round uint64) (Data, error) {

	maxTimeStamp, err := s.maxTimeStamp(ctx, round)
	if err != nil {
		return Data{}, err
	}

	filterFunc, err := s.getBatchFilterFunc(round)
	if err != nil {
		return Data{}, fmt.Errorf("get batches with balance below value: %w", err)
	}

	sample, err := s.reserveSample(ctx, salt, depth, maxTimeStamp, filterFunc)
	return sample.Data, err
}

// Reserve generates the sample of reserve storage of a node required for the
// storage incentives agent to participate in the lottery round. In order to generate
// this  we need to iterate through all the chunks in the node's reserve and
// calculate the transformed hashes of all the chunks using the anchor as the salt.
// In order to generate the transformed hashes, we use bmt hash with a prefixed basehash
// keccak256 with anchor as the prefix. Nodes need to calculate the
// in the most optimal way and there are time restrictions. The lottery round is a
// time based round, so nodes participating in the round need to perform this
// calculation within the round limits.
// In order to optimize this we use a simple pipeline pattern:
// Iterate chunk addresses -> Get the chunk data and calculate transformed hash -> Assemble the
func (s *Sampler) reserveSample(
	ctx context.Context,
	anchor []byte,
	depth uint8,
	maxTimeStamp uint64,
	filterFunc func(batchID []byte) bool,
) (Sample, error) {
	g, ctx := errgroup.WithContext(ctx)
	chunkC := make(chan storer.Chunk, 64)
	// all performance stats should be removed, this is a benchmarking exercise, any interal data is not really meaningful
	// as it is presented to the user
	allStats := &Stats{}
	statsLock := sync.Mutex{}
	addStats := func(stats Stats) {
		statsLock.Lock()
		allStats.add(stats)
		statsLock.Unlock()
	}
	start := time.Now()

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		start := time.Now()
		stats := Stats{}
		defer func() {
			stats.IterationDuration = time.Since(start)
			close(chunkC)
			addStats(stats)
		}()

		err := s.sampler.Iterate(depth, func(c storer.Chunk) (bool, error) {
			select {
			case chunkC <- c:
				stats.TotalIterated++
				return false, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		})
		return err
	})

	// Phase 2: Get the chunk data and calculate transformed hash
	ItemChan := make(chan Item, 64)

	prefixHasherFactory := func() hash.Hash {
		return swarm.NewPrefixHasher(anchor)
	}
	transHasherPool := bmt.NewPool(bmt.NewConf(prefixHasherFactory, swarm.BmtBranches, bmtpool.Capacity))

	const workers = 6

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			wstat := Stats{}
			defer func() {
				addStats(wstat)
			}()

			transProver := bmt.Prover{Hasher: transHasherPool.Get()}
			for c := range chunkC {
				// exclude chunks whose batches balance are below minimum
				if filterFunc(c.BatchID) {
					wstat.BelowBalanceIgnored++
					continue
				}

				chunkLoadStart := time.Now()

				chunk, err := c.Chunk(ctx)
				if err != nil {
					wstat.ChunkLoadFailed++
					return fmt.Errorf("failed loading chunk at address=%x: %w", c.Address, err)
				}

				wstat.ChunkLoadDuration += time.Since(chunkLoadStart)

				var sch *soc.SOC
				if c.Type == swarm.ChunkTypeSingleOwner {
					sch, err = soc.FromChunk(chunk)
					if err != nil {
						return err
					}
				}

				taddrStart := time.Now()
				taddr, err := transformedAddress(transProver, chunk, c.Type)
				if err != nil {
					return err
				}
				wstat.TaddrDuration += time.Since(taddrStart)

				select {
				case ItemChan <- Item{
					TransAddress: taddr,
					Address:      c.Address,
					Stamp:        postage.NewStamp(c.BatchID, nil, nil, nil),
					LoadStamp:    c.Stamp,
					TransProver:  transProver,
					SOC:          sch,
				}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			return nil
		})
	}

	go func() {
		_ = g.Wait()
		close(ItemChan)
	}()

	Items := make([]Item, 0, Size)
	// insert function will insert the new item in its correct place. If the
	// size goes beyond what we need we omit the last item.
	insert := func(item Item) {
		added := false
		for i, sItem := range Items {
			if le(item.TransAddress, sItem.TransAddress) {
				Items = append(Items[:i+1], Items[i:]...)
				Items[i] = item
				added = true
				break
			}
		}
		if len(Items) > Size {
			Items = Items[:Size]
		}
		if len(Items) < Size && !added {
			Items = append(Items, item)
		}
	}

	// Phase 3: Assemble the . Here we need to assemble only the first Size
	// no of items from the results of the 2nd phase.
	// In this step stamps are loaded and validated only if chunk will be added to .
	stats := Stats{}
	for item := range ItemChan {
		currentMaxAddr := swarm.EmptyAddress
		if len(Items) > 0 {
			currentMaxAddr = Items[len(Items)-1].TransAddress
		}

		if len(Items) >= Size && le(currentMaxAddr, item.TransAddress) {
			continue
		}

		stamp, err := item.LoadStamp()
		if err != nil {
			stats.StampLoadFailed++
			// db.logger.Debug("failed loading stamp", "chunk_address", item.ChunkAddress, "error", err)
			continue
		}

		// if _, err := db.validStamp(ch); err != nil {
		// 	stats.InvalidStamp++
		// 	db.logger.Debug("invalid stamp for chunk", "chunk_address", ch.Address(), "error", err)
		// 	continue
		// }

		if binary.BigEndian.Uint64(stamp.Timestamp()) > maxTimeStamp {
			stats.NewIgnored++
			continue
		}
		item.Stamp = stamp

		insert(item)
		stats.Inserts++
	}
	addStats(stats)

	allStats.TotalDuration = time.Since(start)

	if err := g.Wait(); err != nil {
		// db.logger.Info("reserve r finished with error", "err", err, "duration", time.Since(t), "storage_radius", storageRadius, "stats", fmt.Sprintf("%+v", allStats))

		return Sample{}, fmt.Errorf("r: failed creating : %w", err)
	}

	// db.logger.Info("reserve r finished", "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", fmt.Sprintf("%+v", allStats))

	data, err := s.sampleChunk(depth, Items)
	if err != nil {
		return Sample{}, err
	}
	return Sample{Stats: *allStats, Data: *data}, nil
}
func (s *Sampler) sampleChunk(depth uint8, items []Item) (*Data, error) {
	size := len(items) * 2 * swarm.HashSize
	content := make([]byte, size)
	for i, s := range items {
		copy(content[i*swarm.HashSize:], s.Address.Bytes())
		copy(content[(i+1)*2*swarm.HashSize:], s.TransAddress.Bytes())
	}
	prover := bmt.Prover{Hasher: bmtpool.Get()}
	prover.SetHeaderInt64(int64(size))
	_, err := prover.Write(content)
	if err != nil {
		return &Data{}, err
	}
	hash, err := prover.Hash(nil)
	if err != nil {
		return &Data{}, err
	}
	return &Data{depth, swarm.NewAddress(hash), prover, items}, nil
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b swarm.Address) bool {
	return bytes.Compare(a.Bytes(), b.Bytes()) == -1
}

func transformedAddress(hasher bmt.Prover, chunk swarm.Chunk, chType swarm.ChunkType) (swarm.Address, error) {
	switch chType {
	case swarm.ChunkTypeContentAddressed:
		return transformedAddressCAC(hasher, chunk)
	case swarm.ChunkTypeSingleOwner:
		return transformedAddressSOC(hasher, chunk)
	default:
		return swarm.ZeroAddress, fmt.Errorf("chunk type [%v] is is not valid", chType)
	}
}

func transformedAddressCAC(hasher bmt.Prover, chunk swarm.Chunk) (swarm.Address, error) {
	hasher.Reset()
	hasher.SetHeader(chunk.Data()[:bmt.SpanSize])

	_, err := hasher.Write(chunk.Data()[bmt.SpanSize:])
	if err != nil {
		return swarm.ZeroAddress, err
	}

	taddr, err := hasher.Hash(nil)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(taddr), nil
}

func transformedAddressSOC(hasher bmt.Prover, chunk swarm.Chunk) (swarm.Address, error) {
	// Calculate transformed address from wrapped chunk
	sChunk, err := soc.FromChunk(chunk)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	taddrCac, err := transformedAddressCAC(hasher, sChunk.WrappedChunk())
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// Hash address and transformed address to make transformed address for this SOC
	sHasher := swarm.NewHasher()
	if _, err := sHasher.Write(chunk.Address().Bytes()); err != nil {
		return swarm.ZeroAddress, err
	}
	if _, err := sHasher.Write(taddrCac.Bytes()); err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(sHasher.Sum(nil)), nil
}

type Stats struct {
	TotalDuration             time.Duration
	TotalIterated             int64
	IterationDuration         time.Duration
	Inserts                   int64
	NewIgnored                int64
	InvalidStamp              int64
	BelowBalanceIgnored       int64
	TaddrDuration             time.Duration
	ValidStampDuration        time.Duration
	BatchesBelowValueDuration time.Duration
	RogueChunk                int64
	ChunkLoadDuration         time.Duration
	ChunkLoadFailed           int64
	StampLoadFailed           int64
}

func (s *Stats) add(other Stats) {
	s.TotalDuration += other.TotalDuration
	s.TotalIterated += other.TotalIterated
	s.IterationDuration += other.IterationDuration
	s.Inserts += other.Inserts
	s.NewIgnored += other.NewIgnored
	s.InvalidStamp += other.InvalidStamp
	s.BelowBalanceIgnored += other.BelowBalanceIgnored
	s.TaddrDuration += other.TaddrDuration
	s.ValidStampDuration += other.ValidStampDuration
	s.BatchesBelowValueDuration += other.BatchesBelowValueDuration
	s.RogueChunk += other.RogueChunk
	s.ChunkLoadDuration += other.ChunkLoadDuration
	s.ChunkLoadFailed += other.ChunkLoadFailed
	s.StampLoadFailed += other.StampLoadFailed
}

func (s *Sampler) StorageRadius() uint8 {
	return s.sampler.StorageRadius()
}

func (s *Sampler) getBatchFilterFunc(round uint64) (func(batchID []byte) bool, error) {
	cs := s.batchstore.GetChainState()
	blocksToLive := (round+2)*152 - cs.Block
	// blocksToLive := (round+2)*blocksPerRound - cs.Block
	minBalance := new(big.Int).Add(cs.TotalAmount, new(big.Int).Mul(cs.CurrentPrice, big.NewInt(int64(blocksToLive))))

	excluded := make(map[string]struct{})
	err := s.batchstore.IterateByValue(func(id []byte, val *big.Int) (bool, error) {
		if val.Cmp(minBalance) > 0 {
			return true, nil
		}
		excluded[string(id)] = struct{}{}
		return false, nil
	})
	return func(id []byte) bool {
		_, found := excluded[string(id)]
		return found
	}, err
}

func (s *Sampler) maxTimeStamp(ctx context.Context, round uint64) (uint64, error) {
	previousRoundBlockNumber := new(big.Int).SetUint64((round - 1) * 152)
	// previousRoundBlockNumber := new(b/ig.Int).SetUint64((round - 1) * blocksPerRound)

	// s.metrics.BackendCalls.Inc()
	lastBlock, err := s.backend.HeaderByNumber(ctx, previousRoundBlockNumber)
	if err != nil {
		// s.metrics.BackendErrors.Inc()
		return 0, err
	}

	return lastBlock.Time, nil
}
