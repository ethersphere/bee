// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"encoding/binary"
	"errors"
	"strings"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	retrievalIndexItemSize = swarm.HashSize + 8 + sharky.LocationSize
	chunkStampItemSize     = 32 + 8 + 8 + 65
)

var (
	// errInvalidRetrievalIndexAddress is returned if the retrievalIndexItem address is zero during marshaling
	errInvalidRetrievalIndexAddress = errors.New("marshal retrievalIndexItem: address is zero")
	// errInvalidRetrievalIndexLocation is returned if the retrievalIndexItem location is invalid during marshaling
	errInvalidRetrievalIndexLocation = errors.New("marshal retrievalIndexItem: location is invalid")
	// errInvalidRetrievalIndexSize is returned during unmarshaling if the passed buffer is not the expected size
	errInvalidRetrievalIndexSize = errors.New("unmarshal retrievalIndexItem: invalid size")
	// errInvalidRetrievalIndexLocationBytes is returned during unmarshaling if the location buffer is invalid
	errInvalidRetrievalIndexLocationBytes = errors.New("unmarshal retrievalIndexItem: invalid location bytes")

	// errInvalidChunkStampBatchID is returned if the BatchID is invalid during marshaling
	errInvalidChunkStampBatchID = errors.New("marshal chunkStampItem: invalid batch ID")
	// errInvalidChunkStampBatchIndex is returned if the Index is invalid during marshaling
	errInvalidChunkStampBatchIndex = errors.New("marshal chunkStampItem: invalid batch index")
	// errInvalidChunkStampSignature is returned if the Signature is invalid during marshaling
	errInvalidChunkStampSignature = errors.New("marshal chunkStampItem: invalid signature")
	// errInvalidChunkStampSize is returned during unmarshaling if the passed buffer is not the expected size
	errInvalidChunkStampSize = errors.New("marshal chunkStampItem: invalid size")
)

// retrievalIndexItem is the index which gives us the sharky location from the swarm.Address
type retrievalIndexItem struct {
	Address   swarm.Address
	Timestamp uint64
	Location  sharky.Location
}

func (retrievalIndexItem) Namespace() string { return "retrievalIdx" }

func (r *retrievalIndexItem) ID() string { return r.Address.ByteString() }

// Stored in bytes as
// |--Address--|--Timestamp--|--Location--|
//      32            8            7
func (r *retrievalIndexItem) Marshal() ([]byte, error) {
	if r.Address.IsZero() {
		return nil, errInvalidRetrievalIndexAddress
	}

	locBuf, err := r.Location.MarshalBinary()
	if err != nil {
		return nil, errInvalidRetrievalIndexLocation
	}

	buf := make([]byte, retrievalIndexItemSize)
	copy(buf, r.Address.Bytes())
	binary.LittleEndian.PutUint64(buf[swarm.HashSize:], r.Timestamp)
	copy(buf[swarm.HashSize+8:], locBuf)
	return buf, nil
}

func (r *retrievalIndexItem) Unmarshal(buf []byte) error {
	if len(buf) != retrievalIndexItemSize {
		return errInvalidRetrievalIndexSize
	}

	loc := new(sharky.Location)
	if err := loc.UnmarshalBinary(buf[swarm.HashSize+8:]); err != nil {
		return errInvalidRetrievalIndexLocationBytes
	}

	ni := new(retrievalIndexItem)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	ni.Timestamp = binary.LittleEndian.Uint64(buf[swarm.HashSize:])
	ni.Location = *loc
	*r = *ni
	return nil
}

// chunkStampItem is the index used to represent a stamp for a chunk. Going ahead we will
// support multiple stamps on chunks. This item will allow mapping multiple stamps to a
// single address. For this reason, the Address is part of the Namespace and can be used
// to iterate on all the stamps for this Address.
type chunkStampItem struct {
	Address   swarm.Address
	BatchID   []byte
	Index     []byte
	Timestamp []byte
	Sig       []byte
}

func (c *chunkStampItem) Namespace() string {
	return strings.Join([]string{"stamp", c.Address.ByteString()}, "/")
}

func (c *chunkStampItem) ID() string {
	return strings.Join([]string{string(c.BatchID), string(c.Index)}, "/")
}

// Address is not part of the payload which is stored as Address is part of the prefix
// hence already known before querying this object. This will be reused during unmarshaling
// Stored in bytes as
// |--BatchID--|--Index--|--Timestamp--|--Signature--|
//      32          8           8             65
func (c *chunkStampItem) Marshal() ([]byte, error) {
	buf := make([]byte, chunkStampItemSize)
	if n := copy(buf, c.BatchID); n != 32 {
		return nil, errInvalidChunkStampBatchID
	}
	if n := copy(buf[32:], c.Index); n != 8 {
		return nil, errInvalidChunkStampBatchIndex
	}
	if n := copy(buf[40:], c.Timestamp); n != 8 {
		return nil, errInvalidChunkStampBatchIndex
	}
	if n := copy(buf[48:], c.Sig); n != 65 {
		return nil, errInvalidChunkStampSignature
	}
	return buf, nil
}

func (c *chunkStampItem) Unmarshal(buf []byte) error {
	if len(buf) != chunkStampItemSize {
		return errInvalidChunkStampSize
	}

	ni := new(chunkStampItem)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), c.Address.Bytes()...))
	ni.BatchID = append(make([]byte, 0, 32), buf[:32]...)
	ni.Index = append(make([]byte, 0, 8), buf[32:40]...)
	ni.Timestamp = append(make([]byte, 0, 8), buf[40:48]...)
	ni.Sig = append(make([]byte, 0, 65), buf[48:]...)
	*c = *ni
	return nil
}
