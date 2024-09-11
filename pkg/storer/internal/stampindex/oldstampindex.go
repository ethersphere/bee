// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampindex

import (
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ItemV1 is an store.Item that represents data relevant to stamp.
type ItemV1 struct {
	// Keys.
	namespace  []byte // The namespace of other related item.
	BatchID    []byte
	StampIndex []byte

	// Values.
	StampTimestamp   []byte
	ChunkAddress     swarm.Address
	ChunkIsImmutable bool
}

// ID implements the storage.Item interface.
func (i ItemV1) ID() string {
	return fmt.Sprintf("%s/%s/%s", string(i.namespace), string(i.BatchID), string(i.StampIndex))
}

// Namespace implements the storage.Item interface.
func (i ItemV1) Namespace() string {
	return "stampIndex"
}

// Marshal implements the storage.Item interface.
func (i ItemV1) Marshal() ([]byte, error) {
	switch {
	case len(i.namespace) == 0:
		return nil, errStampItemMarshalScopeInvalid
	case len(i.BatchID) != swarm.HashSize:
		return nil, errStampItemMarshalBatchIDInvalid
	case len(i.StampIndex) != swarm.StampIndexSize:
		return nil, errStampItemMarshalBatchIndexInvalid
	}

	buf := make([]byte, 8+len(i.namespace)+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize+swarm.HashSize)

	l := 0
	binary.LittleEndian.PutUint64(buf[l:l+8], uint64(len(i.namespace)))
	l += 8
	copy(buf[l:l+len(i.namespace)], i.namespace)
	l += len(i.namespace)
	copy(buf[l:l+swarm.HashSize], i.BatchID)
	l += swarm.HashSize
	copy(buf[l:l+swarm.StampIndexSize], i.StampIndex)
	l += swarm.StampIndexSize
	copy(buf[l:l+swarm.StampTimestampSize], i.StampTimestamp)
	l += swarm.StampTimestampSize
	copy(buf[l:l+swarm.HashSize], internal.AddressBytesOrZero(i.ChunkAddress))
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (i *ItemV1) Unmarshal(bytes []byte) error {
	if len(bytes) < 8 {
		return errStampItemUnmarshalInvalidSize
	}
	nsLen := int(binary.LittleEndian.Uint64(bytes))
	if len(bytes) != 8+nsLen+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize+swarm.HashSize {
		return errStampItemUnmarshalInvalidSize
	}

	ni := new(ItemV1)
	l := 8
	ni.namespace = append(make([]byte, 0, nsLen), bytes[l:l+nsLen]...)
	l += nsLen
	ni.BatchID = append(make([]byte, 0, swarm.HashSize), bytes[l:l+swarm.HashSize]...)
	l += swarm.HashSize
	ni.StampIndex = append(make([]byte, 0, swarm.StampIndexSize), bytes[l:l+swarm.StampIndexSize]...)
	l += swarm.StampIndexSize
	ni.StampTimestamp = append(make([]byte, 0, swarm.StampTimestampSize), bytes[l:l+swarm.StampTimestampSize]...)
	l += swarm.StampTimestampSize
	ni.ChunkAddress = internal.AddressOrZero(bytes[l : l+swarm.HashSize])
	*i = *ni
	return nil
}

// Clone  implements the storage.Item interface.
func (i *ItemV1) Clone() storage.Item {
	if i == nil {
		return nil
	}
	return &ItemV1{
		namespace:        append([]byte(nil), i.namespace...),
		BatchID:          append([]byte(nil), i.BatchID...),
		StampIndex:       append([]byte(nil), i.StampIndex...),
		StampTimestamp:   append([]byte(nil), i.StampTimestamp...),
		ChunkAddress:     i.ChunkAddress.Clone(),
		ChunkIsImmutable: i.ChunkIsImmutable,
	}
}

// String implements the fmt.Stringer interface.
func (i ItemV1) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

func (i *ItemV1) SetNamespace(ns []byte) {
	i.namespace = ns
}
