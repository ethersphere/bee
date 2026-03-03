// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"context"
	"fmt"
	"hash"
	"math/big"
	"sort"
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
)

const SampleSize = 16

type (
	SampleItem = reserve.SampleItem
	Sample     = reserve.Sample
)

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
// chunks of the selected neighborhood as determined by the anchor and the "committed depth" and NOT the whole reserve.
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
	allStats := &reserve.SampleStats{}
	stamperGetter := func(addr swarm.Address, batchID []byte) (swarm.Stamp, error) {
		return chunkstamp.LoadWithBatchID(db.storage.IndexStore(), "reserve", addr, batchID)
	}

	validStamp := func(ch swarm.Chunk) error {
		_, err := db.validStamp(ch)
		return err
	}

	excludedBatchIDs, err := db.batchesBelowValue(minBatchBalance)
	if err != nil {
		db.logger.Error(err, "get batches below value")
	}
	sampleItems, err := db.reserve.IterateSampleChunks(ctx, db.StorageRadius(), anchor, committedDepth, excludedBatchIDs, consensusTime, db.ChunkStoreGetInto(), stamperGetter, validStamp)
	if err != nil {
		return Sample{}, err
	}

	return Sample{Stats: *allStats, Items: sampleItems.Items}, nil
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

//func (s *SampleStats) add(other SampleStats) {
//s.TotalDuration += other.TotalDuration
//s.IterationDuration += other.IterationDuration
//s.SampleInserts += other.SampleInserts
//s.NewIgnored += other.NewIgnored
//s.InvalidStamp += other.InvalidStamp
//s.BelowBalanceIgnored += other.BelowBalanceIgnored
//s.TaddrDuration += other.TaddrDuration
//s.ValidStampDuration += other.ValidStampDuration
//s.BatchesBelowValueDuration += other.BatchesBelowValueDuration
//s.RogueChunk += other.RogueChunk
//s.ChunkLoadDuration += other.ChunkLoadDuration
//s.ChunkLoadFailed += other.ChunkLoadFailed
//s.StampLoadFailed += other.StampLoadFailed
//s.TotalIterated += other.TotalIterated
//}

// RandSample returns Sample with random values.
func RandSample(t *testing.T, anchor []byte) Sample {
	t.Helper()

	chunks := make([]swarm.Chunk, SampleSize)
	for i := range SampleSize {
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

func (db *DB) recordReserveSampleMetrics(duration time.Duration, stats *reserve.SampleStats, workers int, err error) {
	status := "success"
	if err != nil {
		status = "failure"
	}
	db.metrics.ReserveSampleDuration.WithLabelValues(status).Observe(duration.Seconds())

	summaryMetrics := map[string]float64{
		"duration_seconds":                     duration.Seconds(),
		"chunks_iterated":                      float64(stats.TotalIterated),
		"chunks_load_failed":                   float64(stats.ChunkLoadFailed),
		"stamp_validations":                    float64(stats.SampleInserts),
		"invalid_stamps":                       float64(stats.InvalidStamp),
		"below_balance_ignored":                float64(stats.BelowBalanceIgnored),
		"workers":                              float64(workers),
		"chunks_per_second":                    float64(stats.TotalIterated) / duration.Seconds(),
		"stamp_validation_duration_seconds":    stats.ValidStampDuration.Seconds(),
		"batches_below_value_duration_seconds": stats.BatchesBelowValueDuration.Seconds(),
	}

	for metric, value := range summaryMetrics {
		db.metrics.ReserveSampleRunSummary.WithLabelValues(metric).Set(value)
	}

	db.metrics.ReserveSampleLastRunTimestamp.Set(float64(time.Now().Unix()))
}
