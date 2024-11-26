// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash"
	"math/big"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/soc"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

const SampleSize = 16

type SampleItem struct {
	TransformedAddress swarm.Address
	ChunkAddress       swarm.Address
	ChunkData          []byte
	Stamp              *postage.Stamp
}

type Sample struct {
	Stats SampleStats
	Items []SampleItem
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
// If the node has doubled their capacity by some factor, sampling process need to only pertain to the
// chunks of the selected neighborhood as determined by the anchor and the "committed depth" and NOT the whole reseve.
// The committed depth is the sum of the radius and the doubling factor.
// For example, the committed depth is 11, but the local node has a doubling factor of 3, so the
// local radius will eventually drop to 8. The sampling must only consider chunks with proximity 11 to the anchor.
func (db *DB) ReserveSample(
	ctx context.Context,
	anchor []byte,
	committedDepth uint8,
	consensusTime uint64,
	minBatchBalance *big.Int,
) (Sample, error) {

	g, ctx := errgroup.WithContext(ctx)

	allStats := &SampleStats{}
	statsLock := sync.Mutex{}
	addStats := func(stats SampleStats) {
		statsLock.Lock()
		allStats.add(stats)
		statsLock.Unlock()
	}

	t := time.Now()

	excludedBatchIDs, err := db.batchesBelowValue(minBatchBalance)
	if err != nil {
		db.logger.Error(err, "get batches below value")
	}

	allStats.BatchesBelowValueDuration = time.Since(t)

	chunkC := make(chan *reserve.ChunkBinItem)

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		start := time.Now()
		stats := SampleStats{}
		defer func() {
			stats.IterationDuration = time.Since(start)
			close(chunkC)
			addStats(stats)
		}()

		err := db.reserve.IterateChunksItems(db.StorageRadius(), func(ch *reserve.ChunkBinItem) (bool, error) {
			if swarm.Proximity(ch.Address.Bytes(), anchor) < committedDepth {
				return false, nil
			}
			select {
			case chunkC <- ch:
				stats.TotalIterated++
				return false, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		})
		return err
	})

	// Phase 2: Get the chunk data and calculate transformed hash
	sampleItemChan := make(chan SampleItem)

	prefixHasherFactory := func() hash.Hash {
		return swarm.NewPrefixHasher(anchor)
	}

	workers := max(4, runtime.NumCPU())
	db.logger.Debug("reserve sampler workers", "count", workers)

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			wstat := SampleStats{}
			hasher := bmt.NewHasher(prefixHasherFactory)
			defer func() {
				addStats(wstat)
			}()

			for chItem := range chunkC {
				// exclude chunks who's batches balance are below minimum
				if _, found := excludedBatchIDs[string(chItem.BatchID)]; found {
					wstat.BelowBalanceIgnored++

					continue
				}

				// Skip chunks if they are not SOC or CAC
				if chItem.ChunkType != swarm.ChunkTypeSingleOwner &&
					chItem.ChunkType != swarm.ChunkTypeContentAddressed {
					wstat.RogueChunk++
					continue
				}

				chunkLoadStart := time.Now()

				chunk, err := db.ChunkStore().Get(ctx, chItem.Address)
				if err != nil {
					wstat.ChunkLoadFailed++
					db.logger.Debug("failed loading chunk", "chunk_address", chItem.Address, "error", err)
					continue
				}

				wstat.ChunkLoadDuration += time.Since(chunkLoadStart)

				taddrStart := time.Now()
				taddr, err := transformedAddress(hasher, chunk, chItem.ChunkType)
				if err != nil {
					return err
				}
				wstat.TaddrDuration += time.Since(taddrStart)

				select {
				case sampleItemChan <- SampleItem{
					TransformedAddress: taddr,
					ChunkAddress:       chunk.Address(),
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
			} else if item.TransformedAddress.Compare(sItem.TransformedAddress) == 0 { // ensuring to pass the check order function of redistribution contract
				// replace the chunk at index if the chunk is CAC
				ch := swarm.NewChunk(item.ChunkAddress, item.ChunkData)
				_, err := soc.FromChunk(ch)
				if err != nil {
					sampleItems[i] = item
				}
				return
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
	// In this step stamps are loaded and validated only if chunk will be added to sample.
	stats := SampleStats{}
	for item := range sampleItemChan {
		currentMaxAddr := swarm.EmptyAddress
		if len(sampleItems) > 0 {
			currentMaxAddr = sampleItems[len(sampleItems)-1].TransformedAddress
		}

		if le(item.TransformedAddress, currentMaxAddr) || len(sampleItems) < SampleSize {
			start := time.Now()

			stamp, err := chunkstamp.LoadWithBatchID(db.storage.IndexStore(), "reserve", item.ChunkAddress, item.Stamp.BatchID())
			if err != nil {
				stats.StampLoadFailed++
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

			item.Stamp = postage.NewStamp(stamp.BatchID(), stamp.Index(), stamp.Timestamp(), stamp.Sig())

			insert(item)
			stats.SampleInserts++
		}
	}
	addStats(stats)

	allStats.TotalDuration = time.Since(t)

	if err := g.Wait(); err != nil {
		db.logger.Info("reserve sampler finished with error", "err", err, "duration", time.Since(t), "storage_radius", committedDepth, "consensus_time_ns", consensusTime, "stats", fmt.Sprintf("%+v", allStats))

		return Sample{}, fmt.Errorf("sampler: failed creating sample: %w", err)
	}

	db.logger.Info("reserve sampler finished", "duration", time.Since(t), "storage_radius", committedDepth, "consensus_time_ns", consensusTime, "stats", fmt.Sprintf("%+v", allStats))

	return Sample{Stats: *allStats, Items: sampleItems}, nil
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b swarm.Address) bool {
	return bytes.Compare(a.Bytes(), b.Bytes()) == -1
}

func (db *DB) batchesBelowValue(until *big.Int) (map[string]struct{}, error) {
	res := make(map[string]struct{})

	if until == nil {
		return res, nil
	}

	err := db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {
		if b.Value.Cmp(until) < 0 {
			res[string(b.ID)] = struct{}{}
		}
		return false, nil
	})

	return res, err
}

func transformedAddress(hasher *bmt.Hasher, chunk swarm.Chunk, chType swarm.ChunkType) (swarm.Address, error) {
	switch chType {
	case swarm.ChunkTypeContentAddressed:
		return transformedAddressCAC(hasher, chunk)
	case swarm.ChunkTypeSingleOwner:
		return transformedAddressSOC(hasher, chunk)
	default:
		return swarm.ZeroAddress, fmt.Errorf("chunk type [%v] is not valid", chType)
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

func transformedAddressSOC(hasher *bmt.Hasher, socChunk swarm.Chunk) (swarm.Address, error) {
	// Calculate transformed address from wrapped chunk
	cacChunk, err := soc.UnwrapCAC(socChunk)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	taddrCac, err := transformedAddressCAC(hasher, cacChunk)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// Hash address and transformed address to make transformed address for this SOC
	sHasher := swarm.NewHasher()
	if _, err := sHasher.Write(socChunk.Address().Bytes()); err != nil {
		return swarm.ZeroAddress, err
	}
	if _, err := sHasher.Write(taddrCac.Bytes()); err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(sHasher.Sum(nil)), nil
}

type SampleStats struct {
	TotalDuration             time.Duration
	TotalIterated             int64
	IterationDuration         time.Duration
	SampleInserts             int64
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

func (s *SampleStats) add(other SampleStats) {
	s.TotalDuration += other.TotalDuration
	s.TotalIterated += other.TotalIterated
	s.IterationDuration += other.IterationDuration
	s.SampleInserts += other.SampleInserts
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

// RandSample returns Sample with random values.
func RandSample(t *testing.T, anchor []byte) Sample {
	t.Helper()

	chunks := make([]swarm.Chunk, SampleSize)
	for i := 0; i < SampleSize; i++ {
		ch := chunk.GenerateTestRandomChunk()
		if i%3 == 0 {
			ch = chunk.GenerateTestRandomSoChunk(t, ch)
		}
		chunks[i] = ch
	}

	sample, err := MakeSampleUsingChunks(chunks, anchor)
	if err != nil {
		t.Fatal(err)
	}

	return sample
}

// MakeSampleUsingChunks returns Sample constructed using supplied chunks.
func MakeSampleUsingChunks(chunks []swarm.Chunk, anchor []byte) (Sample, error) {
	prefixHasherFactory := func() hash.Hash {
		return swarm.NewPrefixHasher(anchor)
	}
	items := make([]SampleItem, len(chunks))
	for i, ch := range chunks {
		tr, err := transformedAddress(bmt.NewHasher(prefixHasherFactory), ch, getChunkType(ch))
		if err != nil {
			return Sample{}, err
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

	return Sample{Items: items}, nil
}

func newStamp(s swarm.Stamp) *postage.Stamp {
	return postage.NewStamp(s.BatchID(), s.Index(), s.Timestamp(), s.Sig())
}

func getChunkType(chunk swarm.Chunk) swarm.ChunkType {
	if cac.Valid(chunk) {
		return swarm.ChunkTypeContentAddressed
	} else if soc.Valid(chunk) {
		return swarm.ChunkTypeSingleOwner
	}
	return swarm.ChunkTypeUnspecified
}
