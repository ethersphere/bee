// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	crand "crypto/rand"
	"encoding/binary"
	"io"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const signatureSize = 65

// MustNewSignature will create a new random signature (65 byte slice). Panics on errors.
func MustNewSignature() []byte {
	sig := make([]byte, signatureSize)
	_, err := io.ReadFull(crand.Reader, sig)
	if err != nil {
		panic(err)
	}
	return sig
}

// MustNewValidSignature will create a new valid signature. Panics on errors.
func MustNewValidSignature(signer crypto.Signer, addr swarm.Address, id, index, timestamp []byte) []byte {
	digest, err := postage.ToSignDigest(addr.Bytes(), id, index, timestamp)
	if err != nil {
		panic(err)
	}

	sig, err := signer.Sign(digest)
	if err != nil {
		panic(err)
	}

	return sig
}

// MustNewStamp will generate a invalid postage stamp with random data. Panics on errors.
func MustNewStamp() *postage.Stamp {
	return postage.NewStamp(MustNewID(), MustNewID()[:8], MustNewID()[:8], MustNewSignature())
}

// MustNewValidStamp will generate a valid postage stamp with random data. Panics on errors.
func MustNewValidStamp(signer crypto.Signer, addr swarm.Address) *postage.Stamp {
	id := MustNewID()
	index := make([]byte, 8)
	copy(index[:4], addr.Bytes()[:4])
	timestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(timestamp, uint64(time.Now().UnixNano()))
	sig := MustNewValidSignature(signer, addr, id, index, timestamp)
	return postage.NewStamp(id, index, timestamp, sig)
}

// MustNewBatchStamp will generate a postage stamp with the provided batch ID and assign
// random data to other fields. Panics on error
func MustNewBatchStamp(batch []byte) *postage.Stamp {
	return postage.NewStamp(batch, MustNewID()[:8], MustNewID()[:8], MustNewSignature())
}

// MustNewBatchStamp will generate a postage stamp with the provided batch ID and assign
// random data to other fields. Panics on error
func MustNewFields(batch []byte, index, ts uint64) *postage.Stamp {
	indexBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(indexBuf, index)
	tsBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBuf, ts)
	return postage.NewStamp(batch, indexBuf, tsBuf, MustNewSignature())
}

// MustNewStampWithTimestamp will generate a postage stamp with provided timestamp and
// random data for other fields. Panics on errors.
func MustNewStampWithTimestamp(ts uint64) *postage.Stamp {
	tsBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBuf, ts)
	return postage.NewStamp(MustNewID(), MustNewID()[:8], tsBuf, MustNewSignature())
}
