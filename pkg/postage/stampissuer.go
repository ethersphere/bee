// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/vmihailenco/msgpack/v5"
)

// stampIssuerData groups related StampIssuer data.
// The data are factored out in order to make
// serialization/deserialization easier and at the same
// time not to export the fields outside of the package.
type stampIssuerData struct {
	Label          string   `msgpack:"label"`          // Label to identify the batch period/importance.
	KeyID          string   `msgpack:"keyID"`          // Owner identity.
	BatchID        []byte   `msgpack:"batchID"`        // The batch stamps are issued from.
	BatchAmount    *big.Int `msgpack:"batchAmount"`    // Amount paid for the batch.
	BatchDepth     uint8    `msgpack:"batchDepth"`     // Batch depth: batch size = 2^{depth}.
	BucketDepth    uint8    `msgpack:"bucketDepth"`    // Bucket depth: the depth of collision Buckets uniformity.
	Buckets        []uint32 `msgpack:"buckets"`        // Collision Buckets: counts per neighbourhoods (limited to 2^{batchdepth-bucketdepth}).
	MaxBucketCount uint32   `msgpack:"maxBucketCount"` // the count of the fullest bucket
	BlockNumber    uint64   `msgpack:"blockNumber"`    // BlockNumber when this batch was created
	ImmutableFlag  bool     `msgpack:"immutableFlag"`  // Specifies immutability of the created batch.
}

// StampIssuer is a local extension of a batch issuing stamps for uploads.
// A StampIssuer instance extends a batch with bucket collision tracking
// embedded in multiple Stampers, can be used concurrently.
type StampIssuer struct {
	bucketMu sync.Mutex
	data     stampIssuerData
}

// NewStampIssuer constructs a StampIssuer as an extension of a batch for local
// upload.
//
// BucketDepth must always be smaller than batchDepth otherwise inc() panics.
func NewStampIssuer(label, keyID string, batchID []byte, batchAmount *big.Int, batchDepth, bucketDepth uint8, blockNumber uint64, immutableFlag bool) *StampIssuer {
	return &StampIssuer{
		data: stampIssuerData{
			Label:         label,
			KeyID:         keyID,
			BatchID:       batchID,
			BatchAmount:   batchAmount,
			BatchDepth:    batchDepth,
			BucketDepth:   bucketDepth,
			Buckets:       make([]uint32, 1<<bucketDepth),
			BlockNumber:   blockNumber,
			ImmutableFlag: immutableFlag,
		},
	}
}

// inc increments the count in the correct collision bucket for a newly stamped
// chunk with address addr.
func (si *StampIssuer) inc(addr swarm.Address) ([]byte, error) {
	si.bucketMu.Lock()
	defer si.bucketMu.Unlock()
	b := toBucket(si.BucketDepth(), addr)
	bucketCount := si.data.Buckets[b]
	if bucketCount == 1<<(si.Depth()-si.BucketDepth()) {
		return nil, ErrBucketFull
	}
	si.data.Buckets[b]++
	if si.data.Buckets[b] > si.data.MaxBucketCount {
		si.data.MaxBucketCount = si.data.Buckets[b]
	}
	return indexToBytes(b, bucketCount), nil
}

// toBucket calculates the index of the collision bucket for a swarm address
// bucket index := collision bucket depth number of bits as bigendian uint32
func toBucket(depth uint8, addr swarm.Address) uint32 {
	i := binary.BigEndian.Uint32(addr.Bytes()[:4])
	return i >> (32 - depth)
}

// indexToBytes creates an uint64 index from
// - bucket index (neighbourhood index, uint32 <2^depth, bytes 2-4)
// - and the within-bucket index (uint32 <2^(batchdepth-bucketdepth), bytes 5-8)
func indexToBytes(bucket, index uint32) []byte {
	buf := make([]byte, IndexSize)
	binary.BigEndian.PutUint32(buf, bucket)
	binary.BigEndian.PutUint32(buf[4:], index)
	return buf
}

func bytesToIndex(buf []byte) (bucket, index uint32) {
	index64 := binary.BigEndian.Uint64(buf)
	bucket = uint32(index64 >> 32)
	index = uint32(index64)
	return bucket, index
}

// Label returns the label of the issuer.
func (si *StampIssuer) Label() string {
	return si.data.Label
}

// MarshalBinary implements the encoding.BinaryMarshaler interface.
func (si *StampIssuer) MarshalBinary() ([]byte, error) {
	return msgpack.Marshal(si.data)
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface.
func (si *StampIssuer) UnmarshalBinary(data []byte) error {
	return msgpack.Unmarshal(data, &si.data)
}

// Utilization returns the batch utilization in the form of
// an integer between 0 and 4294967295. Batch fullness can be
// calculated with: max_bucket_value / 2 ^ (batch_depth - bucket_depth)
func (si *StampIssuer) Utilization() uint32 {
	si.bucketMu.Lock()
	defer si.bucketMu.Unlock()
	return si.data.MaxBucketCount
}

// ID returns the BatchID for this batch.
func (si *StampIssuer) ID() []byte {
	id := make([]byte, len(si.data.BatchID))
	copy(id, si.data.BatchID)
	return id
}

// Depth represent issued batch depth.
func (si *StampIssuer) Depth() uint8 {
	return si.data.BatchDepth
}

// Amount represent issued batch amount paid.
func (si *StampIssuer) Amount() *big.Int {
	return si.data.BatchAmount
}

// BucketDepth the depth of collision Buckets uniformity.
func (si *StampIssuer) BucketDepth() uint8 {
	return si.data.BucketDepth
}

// BucketUpperBound returns the maximum number of collisions
// possible in a bucket given the batch's depth and bucket
// depth.
func (si *StampIssuer) BucketUpperBound() uint32 {
	return 1 << (si.Depth() - si.BucketDepth())
}

// BlockNumber when this batch was created.
func (si *StampIssuer) BlockNumber() uint64 {
	return si.data.BlockNumber
}

// ImmutableFlag immutability of the created batch.
func (si *StampIssuer) ImmutableFlag() bool {
	return si.data.ImmutableFlag
}

func (si *StampIssuer) Buckets() []uint32 {
	si.bucketMu.Lock()
	b := make([]uint32, len(si.data.Buckets))
	copy(b, si.data.Buckets)
	si.bucketMu.Unlock()
	return b
}
