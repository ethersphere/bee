// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"math/big"
)

// batchSize is the number of bytes in the serialisation of a batch
const batchSize = 95

// Batch represents a postage batch, a payment on the blockchain.
type Batch struct {
	ID          []byte   // batch ID
	Value       *big.Int // normalised balance of the batch
	Start       uint64   // block number the batch was created
	Owner       []byte   // owner's ethereum address
	Depth       uint8    // batch depth, i.e., size = 2^{depth}
	BucketDepth uint8    // the depth of neighbourhoods t
	Immutable   bool     // if the batch allows adding new capacity (dilution)
}

// MarshalBinary implements BinaryMarshaller. It will attempt to serialize the
// postage batch to a byte slice.
// serialised as ID(32)|big endian value(32)|start block(8)|owner addr(20)|BucketDepth(1)|depth(1)|immutable(1)
func (b *Batch) MarshalBinary() ([]byte, error) {
	if b.Value == nil {
		return nil, ErrBatchInvalid
	}
	value := b.Value.Bytes()
	// the fields are copied into fixed size windows; an oversized field would
	// either silently overwrite a neighbouring field or, for the value, index
	// out of range.
	if len(value) > 32 || len(b.ID) > 32 || len(b.Owner) > 20 {
		return nil, ErrBatchInvalid
	}
	out := make([]byte, batchSize)
	copy(out, b.ID)
	copy(out[64-len(value):], value)
	binary.BigEndian.PutUint64(out[64:72], b.Start)
	copy(out[72:], b.Owner)
	out[92] = b.BucketDepth
	out[93] = b.Depth
	if b.Immutable {
		out[94] = 1
	}
	return out, nil
}

// UnmarshalBinary implements BinaryUnmarshaller. It will attempt deserialize
// the given byte slice into the batch.
func (b *Batch) UnmarshalBinary(buf []byte) error {
	// a shorter buffer cannot be deserialised. longer buffers are tolerated:
	// bee <v2.0.0 serialised batches as 96 bytes (with a trailing storage
	// radius byte) and such records may still be present in the state store,
	// where they have always been read as the first 95 bytes.
	if len(buf) < batchSize {
		return ErrBatchInvalid
	}
	b.ID = buf[:32]
	b.Value = big.NewInt(0).SetBytes(buf[32:64])
	b.Start = binary.BigEndian.Uint64(buf[64:72])
	b.Owner = buf[72:92]
	b.BucketDepth = buf[92]
	b.Depth = buf[93]
	b.Immutable = buf[94] > 0
	return nil
}
