// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

type SampleItem struct {
	TransformedAddress swarm.Address
	ChunkAddress       swarm.Address
	ChunkData          []byte
	Stamp              swarm.Stamp
}

type Sample struct {
	Items []SampleItem
}

func RandSample(t *testing.T, anchor []byte) Sample {
	t.Helper()

	hasher := bmt.NewTrHasher(anchor)

	items := make([]SampleItem, SampleSize)
	for i := 0; i < SampleSize; i++ {
		ch := chunk.GenerateTestRandomChunk()

		tr, err := transformedAddress(hasher, ch, swarm.ChunkTypeContentAddressed)
		if err != nil {
			t.Fatal(err)
		}

		items[i] = SampleItem{
			TransformedAddress: tr,
			ChunkAddress:       ch.Address(),
			ChunkData:          ch.Data(),
			Stamp:              newStamp(ch.Stamp()),
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].TransformedAddress.Compare(items[j].TransformedAddress) == -1
	})

	return Sample{Items: items}
}

func newStamp(s swarm.Stamp) *postage.Stamp {
	return postage.NewStamp(s.BatchID(), s.Index(), s.Timestamp(), s.Sig())
}

// ReserveSample generates the sample of reserve storage of a node required for the
// storage incentives agent to participate in the lottery round. In order to generate
// this sample we need to iterate through all the chunks in the node's reserve and
// calculate the transformed hashes of all the chunks using the anchor as the salt.
// In order to generate the transformed hashes, we will use the std hmac keyed-hash
// implementation by using the anchor as the key. Nodes need to calculate the sample
// in the most optimal way and there are time restrictions. The lottery round is a
// time based round, so nodes participating in the round need to perform this
// calculation within the round limits.
// In order to optimize this we use a simple pipeline pattern:
// Iterate chunk addresses -> Get the chunk data and calculate transformed hash -> Assemble the sample
func (db *DB) ReserveSample(
	ctx context.Context,
	anchor []byte,
	storageRadius uint8,
	consensusTime uint64,
	minBatchBalance *big.Int,
) (Sample, error) {
	g, ctx := errgroup.WithContext(ctx)
	chunkC := make(chan reserve.ChunkItem, 64)
	allStats := &sampleStat{}
	statsLock := sync.Mutex{}

	excludedBatchIDs, err := db.batchesBelowValue(minBatchBalance)
	if err != nil {
		db.logger.Error(err, "get batches below value")
	}

	t := time.Now()

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		start := time.Now()
		stats := sampleStat{}
		defer func() {
			stats.IterationDuration = time.Since(start)
			close(chunkC)

			statsLock.Lock()
			allStats.add(stats)
			statsLock.Unlock()
		}()

		err := db.reserve.IterateChunksItems(db.repo, storageRadius, func(chi reserve.ChunkItem) (bool, error) {
			select {
			case chunkC <- chi:
				stats.TotalIterated++
				return false, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		})
		return err
	})

	// Phase 2: Get the chunk data and calculate transformed hash
	sampleItemChan := make(chan SampleItem, 64)

	const workers = 6
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			wstat := sampleStat{}
			defer func() {
				statsLock.Lock()
				allStats.add(wstat)
				statsLock.Unlock()
			}()

			hmacr := hmac.New(swarm.NewHasher, anchor)
			for chItem := range chunkC {
				// exclude chunks who's batches balance are below minimum
				if _, found := excludedBatchIDs[string(chItem.BatchID)]; found {
					wstat.BelowBalanceIgnored++
					continue
				}

				// Skip chunks if they are not SOC or CAC
				if chItem.Type != swarm.ChunkTypeSingleOwner &&
					chItem.Type != swarm.ChunkTypeContentAddressed {
					continue
				}

				chunk, err := db.ChunkStore().Get(ctx, chItem.ChunkAddress)
				if err != nil {
					db.logger.Debug("failed loading chunk", "chunk_address", chItem.ChunkAddress, "error", err)
					continue
				}

				hmacrStart := time.Now()

				hmacr.Reset()
				_, err = hmacr.Write(chunk.Data())
				if err != nil {
					return err
				}
				taddr := swarm.NewAddress(hmacr.Sum(nil))

				wstat.HmacrDuration += time.Since(hmacrStart)

				select {
				case sampleItemChan <- SampleItem{
					TransformedAddress: taddr,
					ChunkAddress:       chItem.ChunkAddress,
					ChunkData:          chunk.Data(),
					Stamp:              postage.NewStamp(chItem.BatchID, nil, nil, nil),
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
		close(sampleItemChan)
	}()

	sampleItems := make([]SampleItem, 0, SampleSize)
	// insert function will insert the new item in its correct place. If the sample
	// size goes beyond what we need we omit the last item.
	insert := func(item SampleItem) {
		added := false
		for i, sItem := range sampleItems {
			if le(item.TransformedAddress, sItem.TransformedAddress) {
				sampleItems = append(sampleItems[:i+1], sampleItems[i:]...)
				sampleItems[i] = item
				added = true
				break
			}
		}
		if len(sampleItems) > SampleSize {
			sampleItems = sampleItems[:SampleSize]
		}
		if len(sampleItems) < SampleSize && !added {
			sampleItems = append(sampleItems, item)
		}
	}

	// Phase 3: Assemble the sample. Here we need to assemble only the first SampleSize
	// no of items from the results of the 2nd phase.
	stats := sampleStat{}
	for item := range sampleItemChan {
		currentMaxAddr := swarm.EmptyAddress
		if len(sampleItems) > 0 {
			currentMaxAddr = sampleItems[len(sampleItems)-1].TransformedAddress
		}

		if le(item.TransformedAddress, currentMaxAddr) || len(sampleItems) < SampleSize {
			start := time.Now()

			stamp, err := chunkstamp.LoadWithBatchID(db.repo.IndexStore(), "reserve", item.ChunkAddress, item.Stamp.BatchID())
			if err != nil {
				db.logger.Debug("failed loading stamp", "chunk_address", item.ChunkAddress, "error", err)
				continue
			}

			ch := swarm.NewChunk(item.ChunkAddress, item.ChunkData).WithStamp(stamp)

			// check if the timestamp on the postage stamp is not later than the consensus time.
			if binary.BigEndian.Uint64(ch.Stamp().Timestamp()) > consensusTime {
				stats.NewIgnored++
				continue
			}

			if _, err := db.validStamp(ch); err != nil {
				stats.InvalidStamp++
				db.logger.Debug("invalid stamp for chunk", "chunk_address", ch.Address(), "error", err)
				continue
			}

			stats.ValidStampDuration += time.Since(start)

			item.Stamp = stamp
			insert(item)
			stats.SampleInserts++
		}
	}
	allStats.add(stats)

	if err := g.Wait(); err != nil {
		db.logger.Info("reserve sampler finished with error", "err", err, "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", allStats)

		return Sample{}, fmt.Errorf("sampler: failed creating sample: %w", err)
	}

	db.logger.Info("reserve sampler finished", "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", allStats)

	return Sample{Items: sampleItems}, nil
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b swarm.Address) bool {
	return bytes.Compare(a.Bytes(), b.Bytes()) == -1
}

func transformedAddress(hasher *bmt.Hasher, chunk swarm.Chunk, chType swarm.ChunkType) (swarm.Address, error) {
	switch chType {
	case swarm.ChunkTypeContentAddressed:
		return transformedAddressCAC(hasher, chunk)
	case swarm.ChunkTypeSingleOwner:
		return transformedAddressSOC(hasher, chunk)
	default:
		return swarm.ZeroAddress, fmt.Errorf("chunk type [%v] is is not valid", chType)
	}
}

func transformedAddressCAC(hasher *bmt.Hasher, chunk swarm.Chunk) (swarm.Address, error) {
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

func transformedAddressSOC(hasher *bmt.Hasher, chunk swarm.Chunk) (swarm.Address, error) {
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

type sampleStat struct {
	TotalIterated       int64
	IterationDuration   time.Duration
	SampleInserts       int64
	NewIgnored          int64
	InvalidStamp        int64
	BelowBalanceIgnored int64
	HmacrDuration       time.Duration
	ValidStampDuration  time.Duration
}

func (s *sampleStat) add(other sampleStat) {
	s.TotalIterated += other.TotalIterated
	s.IterationDuration += other.IterationDuration
	s.SampleInserts += other.SampleInserts
	s.NewIgnored += other.NewIgnored
	s.InvalidStamp += other.InvalidStamp
	s.BelowBalanceIgnored += other.BelowBalanceIgnored
	s.HmacrDuration += other.HmacrDuration
	s.ValidStampDuration += other.ValidStampDuration
}

func (s sampleStat) String() string {
	return fmt.Sprintf(
		"TotalChunks: %d SampleInserts: %d NewIgnored: %d InvalidStamp: %d BelowBalanceIgnored: %d "+
			"IterationDuration: %s HmacrDuration: %s ValidStampDuration: %s",
		s.TotalIterated,
		s.SampleInserts,
		s.NewIgnored,
		s.InvalidStamp,
		s.BelowBalanceIgnored,
		s.IterationDuration,
		s.HmacrDuration,
		s.ValidStampDuration,
	)
}
