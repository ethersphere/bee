// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"path"
	"sync"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/vmihailenco/msgpack/v5"
)

var (
	// errStampItemMarshalBatchIDInvalid is returned when trying to
	// marshal a stampItem with invalid batchID.
	errStampItemMarshalBatchIDInvalid = errors.New("marshal postage.stampItem: batchID is invalid")
	// errStampItemMarshalChunkAddressInvalid is returned when trying
	// to marshal a stampItem with invalid chunkAddress.
	errStampItemMarshalChunkAddressInvalid = errors.New("marshal postage.stampItem: chunkAddress is invalid")
	// errStampItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer with smaller size then is the size
	// of the Item fields.
	errStampItemUnmarshalInvalidSize = errors.New("unmarshal postage.stampItem: invalid size")
)

const stampItemSize = swarm.HashSize + swarm.HashSize + swarm.StampIndexSize + swarm.StampTimestampSize

type StampItem struct {
	// Keys.
	BatchID      []byte
	chunkAddress swarm.Address

	// Values.
	BatchIndex     []byte
	BatchTimestamp []byte
}

// ID implements the storage.Item interface.
func (si StampItem) ID() string {
	return fmt.Sprintf("%s/%s", string(si.BatchID), si.chunkAddress.String())
}

// Namespace implements the storage.Item interface.
func (si StampItem) Namespace() string {
	return "stampItem"
}

// Marshal implements the storage.Item interface.
func (si StampItem) Marshal() ([]byte, error) {
	switch {
	case len(si.BatchID) != swarm.HashSize:
		return nil, errStampItemMarshalBatchIDInvalid
	case len(si.chunkAddress.Bytes()) != swarm.HashSize:
		return nil, errStampItemMarshalChunkAddressInvalid
	}

	buf := make([]byte, stampItemSize+1)

	l := 0
	copy(buf[l:l+swarm.HashSize], si.BatchID)
	l += swarm.HashSize
	copy(buf[l:l+swarm.HashSize], si.chunkAddress.Bytes())
	l += swarm.HashSize
	copy(buf[l:l+swarm.StampIndexSize], si.BatchIndex)
	l += swarm.StampIndexSize
	copy(buf[l:l+swarm.StampTimestampSize], si.BatchTimestamp)

	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (si *StampItem) Unmarshal(bytes []byte) error {
	if len(bytes) != stampItemSize+1 {
		return errStampItemUnmarshalInvalidSize
	}

	ni := new(StampItem)

	l := 0
	ni.BatchID = append(make([]byte, 0, swarm.HashSize), bytes[l:l+swarm.HashSize]...)
	l += swarm.HashSize
	ni.chunkAddress = swarm.NewAddress(bytes[l : l+swarm.HashSize])
	l += swarm.HashSize
	ni.BatchIndex = append(make([]byte, 0, swarm.StampIndexSize), bytes[l:l+swarm.StampIndexSize]...)
	l += swarm.StampIndexSize
	ni.BatchTimestamp = append(make([]byte, 0, swarm.StampTimestampSize), bytes[l:l+swarm.StampTimestampSize]...)

	*si = *ni
	return nil
}

// Clone  implements the storage.Item interface.
func (si *StampItem) Clone() storage.Item {
	if si == nil {
		return nil
	}
	return &StampItem{
		BatchID:        append([]byte(nil), si.BatchID...),
		chunkAddress:   si.chunkAddress.Clone(),
		BatchIndex:     append([]byte(nil), si.BatchIndex...),
		BatchTimestamp: append([]byte(nil), si.BatchTimestamp...),
	}
}

// String implements the fmt.Stringer interface.
func (si StampItem) String() string {
	return path.Join(si.Namespace(), si.ID())
}

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

// Clone returns a deep copy of the stampIssuerData.
func (s stampIssuerData) Clone() stampIssuerData {
	return stampIssuerData{
		Label:         s.Label,
		KeyID:         s.KeyID,
		BatchID:       append([]byte(nil), s.BatchID...),
		BatchAmount:   new(big.Int).Set(s.BatchAmount),
		BatchDepth:    s.BatchDepth,
		BucketDepth:   s.BucketDepth,
		Buckets:       append([]uint32(nil), s.Buckets...),
		BlockNumber:   s.BlockNumber,
		ImmutableFlag: s.ImmutableFlag,
	}
}

// StampIssuer is a local extension of a batch issuing stamps for uploads.
// A StampIssuer instance extends a batch with bucket collision tracking
// embedded in multiple Stampers, can be used concurrently.
type StampIssuer struct {
	data stampIssuerData
	mtx  sync.Mutex
}

// NewStampIssuer constructs a StampIssuer as an extension of a batch for local
// upload.
//
// BucketDepth must always be smaller than batchDepth otherwise increment() panics.
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

// increment increments the count in the correct collision
// bucket for a newly stamped chunk with given addr address.
// Must be mutex locked before usage.
func (si *StampIssuer) increment(addr swarm.Address) (batchIndex []byte, batchTimestamp []byte, err error) {
	bIdx := toBucket(si.BucketDepth(), addr)
	bCnt := si.data.Buckets[bIdx]

	if bCnt == si.BucketUpperBound() {
		if si.ImmutableFlag() {
			return nil, nil, ErrBucketFull
		}

		bCnt = 0
		si.data.Buckets[bIdx] = 0
	}

	si.data.Buckets[bIdx]++
	if si.data.Buckets[bIdx] > si.data.MaxBucketCount {
		si.data.MaxBucketCount = si.data.Buckets[bIdx]
	}

	return indexToBytes(bIdx, bCnt), unixTime(), nil
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
	si.mtx.Lock()
	defer si.mtx.Unlock()
	b := make([]uint32, len(si.data.Buckets))
	copy(b, si.data.Buckets)
	return b
}

// StampIssuerItem is a storage.Item implementation for StampIssuer.
type StampIssuerItem struct {
	Issuer *StampIssuer
}

// NewStampIssuerItem creates a new StampIssuerItem.
func NewStampIssuerItem(ID []byte) *StampIssuerItem {
	return &StampIssuerItem{
		Issuer: &StampIssuer{
			data: stampIssuerData{
				BatchID: ID,
			},
		},
	}
}

// ID is the batch ID.
func (s *StampIssuerItem) ID() string {
	return string(s.Issuer.ID())
}

// Namespace returns the storage namespace for a stampIssuer.
func (s *StampIssuerItem) Namespace() string {
	return "StampIssuerItem"
}

// Marshal marshals the StampIssuerItem into a byte slice.
func (s *StampIssuerItem) Marshal() ([]byte, error) {
	return s.Issuer.MarshalBinary()
}

// Unmarshal unmarshals a byte slice into a StampIssuerItem.
func (s *StampIssuerItem) Unmarshal(bytes []byte) error {
	issuer := new(StampIssuer)
	err := issuer.UnmarshalBinary(bytes)
	if err != nil {
		return err
	}
	s.Issuer = issuer
	return nil
}

// Clone returns a clone of StampIssuerItem.
func (s *StampIssuerItem) Clone() storage.Item {
	if s == nil {
		return nil
	}
	return &StampIssuerItem{
		Issuer: &StampIssuer{
			data: s.Issuer.data.Clone(),
		},
	}
}

// String returns the string representation of a StampIssuerItem.
func (s StampIssuerItem) String() string {
	return path.Join(s.Namespace(), s.ID())
}

var _ storage.Item = (*StampIssuerItem)(nil)

// toBucket calculates the index of the collision bucket for a swarm address
// bucket index := collision bucket depth number of bits as bigendian uint32
func toBucket(depth uint8, addr swarm.Address) uint32 {
	return binary.BigEndian.Uint32(addr.Bytes()[:4]) >> (32 - depth)
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

// BucketIndexFromBytes returns bucket index and within-bucket index from supplied bytes.
func BucketIndexFromBytes(buf []byte) (bucket, index uint32) {
	index64 := IndexFromBytes(buf)
	return uint32(index64 >> 32), uint32(index64)
}

// IndexFromBytes returns uint64 value from supplied bytes
func IndexFromBytes(buf []byte) uint64 {
	return binary.BigEndian.Uint64(buf)
}

func unixTime() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(time.Now().UnixNano()))
	return buf
}

// TimestampFromBytes returns uint64 value from supplied bytes
func TimestampFromBytes(buf []byte) uint64 {
	return binary.BigEndian.Uint64(buf)
}
