// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"math/big"
)

// Batch represents a postage batch, a payment on the blockchain.
type Batch struct {
	ID    []byte   // batch ID
	Value *big.Int // overall balance of the batch
	Start uint64   // block number the batch was created
	Owner []byte   // owner's ethereum address
	Depth uint8    // batch depth, i.e., size = 2^{depth}
}

// MarshalBinary serialises a postage batch to a byte slice len 117.
func (b *Batch) MarshalBinary() ([]byte, error) {
	out := make([]byte, 93)
	copy(out, b.ID)
	value := b.Value.Bytes()
	copy(out[64-len(value):], value)
	binary.BigEndian.PutUint64(out[64:72], b.Start)
	copy(out[72:], b.Owner)
	out[92] = b.Depth
	return out, nil
}

// UnmarshalBinary deserialises the batch.
// Unsafe on slice index (len(buf) = 117) as only internally used in db.
func (b *Batch) UnmarshalBinary(buf []byte) error {
	b.ID = buf[:32]
	b.Value = big.NewInt(0).SetBytes(buf[32:64])
	b.Start = binary.BigEndian.Uint64(buf[64:72])
	b.Owner = buf[72:92]
	b.Depth = buf[92]
	return nil
}
