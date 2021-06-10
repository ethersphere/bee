// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

// StampIssuer is a local extension of a batch issuing stamps for uploads.
// A StampIssuer instance extends a batch with bucket collision tracking
// embedded in multiple Stampers, can be used concurrently.
type StampIssuer struct {
	label       string     // Label to identify the batch period/importance.
	keyID       string     // Owner identity.
	batchID     []byte     // The batch stamps are issued from.
	batchDepth  uint8      // Batch depth: batch size = 2^{depth}.
	batchAmount *big.Int   // Amount paid for the batch.
	bucketDepth uint8      // Bucket depth: the depth of collision buckets uniformity.
	mu          sync.Mutex // Mutex for buckets.
	buckets     []uint32   // Collision buckets: counts per neighbourhoods (limited to 2^{batchdepth-bucketdepth}).
	createdAt   int64      // Issuer created timestamp..
}

// NewStampIssuer constructs a StampIssuer as an extension of a batch for local
// upload.
//
// bucketDepth must always be smaller than batchDepth otherwise inc() panics.
func NewStampIssuer(label, keyID string, batchID []byte, batchDepth uint8, batchAmount *big.Int, bucketDepth uint8) *StampIssuer {
	return &StampIssuer{
		label:       label,
		keyID:       keyID,
		batchID:     batchID,
		batchDepth:  batchDepth,
		batchAmount: batchAmount,
		bucketDepth: bucketDepth,
		buckets:     make([]uint32, 1<<bucketDepth),
		createdAt:   time.Now().UnixNano(),
	}
}

// inc increments the count in the correct collision bucket for a newly stamped
// chunk with address addr.
func (si *StampIssuer) inc(addr swarm.Address) error {
	si.mu.Lock()
	defer si.mu.Unlock()
	b := toBucket(si.bucketDepth, addr)
	if si.buckets[b] == 1<<(si.batchDepth-si.bucketDepth) {
		return ErrBucketFull
	}
	si.buckets[b]++
	return nil
}

// toBucket calculates the index of the collision bucket for a swarm address
// using depth as collision bucket depth
func toBucket(depth uint8, addr swarm.Address) uint32 {
	i := binary.BigEndian.Uint32(addr.Bytes()[:4])
	return i >> (32 - depth)
}

// Label returns the label of the issuer.
func (si *StampIssuer) Label() string {
	return si.label
}

// MarshalBinary gives the byte slice serialisation of a StampIssuer:
// = label[32]|keyID[32]|batchID[32]|batchDepth[1]|bucketDepth[1]|size_0[4]|size_1[4]|....
func (si *StampIssuer) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 32+32+32+1+1+(1<<(si.bucketDepth+2)))
	label := []byte(si.label)
	copy(buf[32-len(label):32], label)
	keyID := []byte(si.keyID)
	copy(buf[64-len(keyID):64], keyID)
	copy(buf[64:96], si.batchID)
	buf[96] = si.batchDepth
	buf[97] = si.bucketDepth
	si.mu.Lock()
	defer si.mu.Unlock()
	for i, addr := range si.buckets {
		offset := 98 + i*4
		binary.BigEndian.PutUint32(buf[offset:offset+4], addr)
	}
	return buf, nil
}

// UnmarshalBinary parses a serialised StampIssuer into the receiver struct.
func (si *StampIssuer) UnmarshalBinary(buf []byte) error {
	si.label = toString(buf[:32])
	si.keyID = toString(buf[32:64])
	si.batchID = buf[64:96]
	si.batchDepth = buf[96]
	si.bucketDepth = buf[97]
	si.buckets = make([]uint32, 1<<si.bucketDepth)
	// not using lock as unmarshal is init
	for i := range si.buckets {
		offset := 98 + i*4
		si.buckets[i] = binary.BigEndian.Uint32(buf[offset : offset+4])
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
func (si *StampIssuer) Utilization() uint32 {
	top := uint32(0)

	for _, v := range si.buckets {
		if v > top {
			top = v
		}
	}

	return top
}

// ID returns the BatchID for this batch.
func (si *StampIssuer) ID() []byte {
	id := make([]byte, len(si.batchID))
	copy(id, si.batchID)
	return id
}

// Depth represent issued batch depth.
func (si *StampIssuer) Depth() uint8 {
	return si.batchDepth
}

// Amount represent issued batch amount paid.
func (si *StampIssuer) Amount() *big.Int {
	return si.batchAmount
}

// CreatedAt represents the time when this issuer was created.
func (si *StampIssuer) CreatedAt() int64 {
	return si.createdAt
}
