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

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
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

type stampItem struct {
	// Keys.
	batchID      []byte
	chunkAddress swarm.Address

	// Values.
	BatchIndex     []byte
	BatchTimestamp []byte
}

// ID implements the storage.Item interface.
func (si stampItem) ID() string {
	return fmt.Sprintf("%s/%s", string(si.batchID), si.chunkAddress.String())
}

// Namespace implements the storage.Item interface.
func (si stampItem) Namespace() string {
	return "stampItem"
}

// Marshal implements the storage.Item interface.
func (si stampItem) Marshal() ([]byte, error) {
	switch {
	case len(si.batchID) != swarm.HashSize:
		return nil, errStampItemMarshalBatchIDInvalid
	case len(si.chunkAddress.Bytes()) != swarm.HashSize:
		return nil, errStampItemMarshalChunkAddressInvalid
	}

	buf := make([]byte, stampItemSize+1)

	l := 0
	copy(buf[l:l+swarm.HashSize], si.batchID)
	l += swarm.HashSize
	copy(buf[l:l+swarm.HashSize], si.chunkAddress.Bytes())
	l += swarm.HashSize
	copy(buf[l:l+swarm.StampIndexSize], si.BatchIndex)
	l += swarm.StampIndexSize
	copy(buf[l:l+swarm.StampTimestampSize], si.BatchTimestamp)

	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (si *stampItem) Unmarshal(bytes []byte) error {
	if len(bytes) != stampItemSize+1 {
		return errStampItemUnmarshalInvalidSize
	}

	ni := new(stampItem)

	l := 0
	ni.batchID = append(make([]byte, 0, swarm.HashSize), bytes[l:l+swarm.HashSize]...)
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
func (si *stampItem) Clone() storage.Item {
	if si == nil {
		return nil
	}
	return &stampItem{
		batchID:        append([]byte(nil), si.batchID...),
		chunkAddress:   si.chunkAddress.Clone(),
		BatchIndex:     append([]byte(nil), si.BatchIndex...),
		BatchTimestamp: append([]byte(nil), si.BatchTimestamp...),
	}
}

// String implements the fmt.Stringer interface.
func (si stampItem) String() string {
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
	Expired        bool     `msgpack:"expired"`        // Specifies the expiry of the batch
}

// StampIssuer is a local extension of a batch issuing stamps for uploads.
// A StampIssuer instance extends a batch with bucket collision tracking
// embedded in multiple Stampers, can be used concurrently.
type StampIssuer struct {
	store storage.Store

	bucketMu sync.Mutex
	data     stampIssuerData
}

// NewStampIssuer constructs a StampIssuer as an extension of a batch for local
// upload.
//
// BucketDepth must always be smaller than batchDepth otherwise increment() panics.
func NewStampIssuer(label, keyID string, batchID []byte, batchAmount *big.Int, batchDepth, bucketDepth uint8, blockNumber uint64, immutableFlag bool) *StampIssuer {
	return &StampIssuer{
		store: inmemstore.New(), // TODO: initialize this through constructor when fully refactoring to the new store v2.
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
func (si *StampIssuer) increment(addr swarm.Address) (batchIndex []byte, batchTimestamp []byte, err error) {
	item := &stampItem{
		batchID:      si.data.BatchID,
		chunkAddress: addr,
	}
	switch err := si.store.Get(item); {
	case errors.Is(err, storage.ErrNotFound):
		break
	case err != nil:
		return nil, nil, fmt.Errorf("load of stamp item failed for %s: %w", item, err)
	default:
		return item.BatchIndex, item.BatchTimestamp, nil
	}

	si.bucketMu.Lock()
	defer si.bucketMu.Unlock()

	bIdx := toBucket(si.BucketDepth(), addr)
	bCnt := si.data.Buckets[bIdx]
	if bCnt == 1<<(si.Depth()-si.BucketDepth()) {
		return nil, nil, ErrBucketFull
	}

	si.data.Buckets[bIdx]++
	if si.data.Buckets[bIdx] > si.data.MaxBucketCount {
		si.data.MaxBucketCount = si.data.Buckets[bIdx]
	}

	item.BatchIndex = indexToBytes(bIdx, bCnt)
	item.BatchTimestamp = unixTime()

	if err := si.store.Put(item); err != nil {
		return nil, nil, err
	}

	return item.BatchIndex, item.BatchTimestamp, nil
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

// Expired returns the expired property of stamp
func (si *StampIssuer) Expired() bool {
	return si.data.Expired
}

// SetExpired is setter for Expired property
func (si *StampIssuer) SetExpired(e bool) {
	si.data.Expired = e
}

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

func bytesToIndex(buf []byte) (bucket, index uint32) {
	index64 := binary.BigEndian.Uint64(buf)
	return uint32(index64 >> 32), uint32(index64)
}

func unixTime() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(time.Now().UnixNano()))
	return buf
}
