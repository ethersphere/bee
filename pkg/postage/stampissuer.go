// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

// StampIssuer is a local extension of a batch issuing stamps for uploads.
// A StampIssuer instance extends a batch with bucket collision tracking
// embedded in multiple Stampers, can be used concurrently.
type StampIssuer struct {
	label          string     // Label to identify the batch period/importance.
	keyID          string     // Owner identity.
	batchID        []byte     // The batch stamps are issued from.
	batchDepth     uint8      // Batch depth: batch size = 2^{depth}.
	bucketDepth    uint8      // Bucket depth: the depth of collision buckets uniformity.
	mu             sync.Mutex // Mutex for buckets.
	buckets        []uint32   // Collision buckets: counts per neighbourhoods (limited to 2^{batchdepth-bucketdepth}).
	maxBucketCount uint32     // the count of the fullest bucket
}

// NewStampIssuer constructs a StampIssuer as an extension of a batch for local
// upload.
//
// bucketDepth must always be smaller than batchDepth otherwise inc() panics.
func NewStampIssuer(label, keyID string, batchID []byte, batchDepth, bucketDepth uint8) *StampIssuer {
	return &StampIssuer{
		label:       label,
		keyID:       keyID,
		batchID:     batchID,
		batchDepth:  batchDepth,
		bucketDepth: bucketDepth,
		buckets:     make([]uint32, 1<<bucketDepth),
	}
}

// inc increments the count in the correct collision bucket for a newly stamped
// chunk with address addr.
func (st *StampIssuer) inc(addr swarm.Address) ([]byte, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	b := toBucket(st.bucketDepth, addr)
	index := st.buckets[b]
	if index == 1<<(st.batchDepth-st.bucketDepth) {
		return nil, ErrBucketFull
	}
	st.buckets[b]++
	if st.buckets[b] > st.maxBucketCount {
		st.maxBucketCount = st.buckets[b]
	}
	return indexToBytes(b, index), nil
}

// toBucket calculates the index of the collision bucket for a swarm address
// bucket index := collision bucket depth number of bits as bigendian uint32
func toBucket(depth uint8, addr swarm.Address) uint32 {
	i := binary.BigEndian.Uint32(addr.Bytes()[:4])
	return i >> (32 - depth)
}

// indexToBytes creates an uint64 index from
// - bucket depth (uint8, 1st byte, <24)
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
func (st *StampIssuer) Label() string {
	return st.label
}

// MarshalBinary gives the byte slice serialisation of a StampIssuer:
// = label[32]|keyID[32]|batchID[32]|batchDepth[1]|bucketDepth[1]|size_0[4]|size_1[4]|....
func (st *StampIssuer) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 32+32+32+1+1+4*(1<<st.bucketDepth))
	label := []byte(st.label)
	copy(buf[32-len(label):32], label)
	keyID := []byte(st.keyID)
	copy(buf[64-len(keyID):64], keyID)
	copy(buf[64:96], st.batchID)
	buf[96] = st.batchDepth
	buf[97] = st.bucketDepth
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, addr := range st.buckets {
		offset := 98 + i*4
		binary.BigEndian.PutUint32(buf[offset:offset+4], addr)
	}
	return buf, nil
}

// UnmarshalBinary parses a serialised StampIssuer into the receiver struct.
func (st *StampIssuer) UnmarshalBinary(buf []byte) error {
	st.label = toString(buf[:32])
	st.keyID = toString(buf[32:64])
	st.batchID = buf[64:96]
	st.batchDepth = buf[96]
	st.bucketDepth = buf[97]
	st.buckets = make([]uint32, 1<<st.bucketDepth)
	// not using lock as unmarshal is init
	for i := range st.buckets {
		offset := 98 + i*4
		st.buckets[i] = binary.BigEndian.Uint32(buf[offset : offset+4])
	}
	return nil
}

func toString(buf []byte) string {
	i := 0
	var c byte
	for i, c = range buf {
		if c != 0 {
			break
		}
	}
	return string(buf[i:])
}

// Utilization returns the batch utilization in the form of
// an integer between 0 and 4294967295. Batch fullness can be
// calculated with: max_bucket_value / 2 ^ (batch_depth - bucket_depth)
func (st *StampIssuer) Utilization() uint32 {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.maxBucketCount
}

// ID returns the BatchID for this batch.
func (s *StampIssuer) ID() []byte {
	id := make([]byte, len(s.batchID))
	copy(id, s.batchID)
	return id
}
