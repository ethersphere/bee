// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampindex

import (
	"encoding/binary"
	"errors"
	"fmt"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// errStampItemMarshalNamespaceInvalid is returned when trying to
	// marshal a Item with invalid namespace.
	errStampItemMarshalNamespaceInvalid = errors.New("marshal stampindex.Item: namespace is invalid")
	// errStampItemMarshalBatchIDInvalid is returned when trying to
	// marshal a Item with invalid batchID.
	errStampItemMarshalBatchIDInvalid = errors.New("marshal stampindex.Item: batchID is invalid")
	// errStampItemMarshalBatchIndexInvalid is returned when trying
	// to marshal a Item with invalid batchIndex.
	errStampItemMarshalBatchIndexInvalid = errors.New("marshal stampindex.Item: batchIndex is invalid")
	// errStampItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer with smaller size then is the size
	// of the Item fields.
	errStampItemUnmarshalInvalidSize = errors.New("unmarshal stampindex.Item: invalid size")
	// errStampItemUnmarshalChunkImmutableInvalid is returned when trying
	// to unmarshal buffer with invalid ChunkIsImmutable value.
	errStampItemUnmarshalChunkImmutableInvalid = errors.New("unmarshal stampindex.Item: chunk immutable is invalid")
)

var _ storage.Item = (*Item)(nil)

// Item is an store.Item that represents data relevant to stamp.
type Item struct {
	// Keys.
	namespace  []byte // The namespace of other related item.
	batchID    []byte
	stampIndex []byte

	// Values.
	StampTimestamp   []byte
	ChunkAddress     swarm.Address
	ChunkIsImmutable bool
}

// ID implements the storage.Item interface.
func (i Item) ID() string {
	return fmt.Sprintf("%s/%s/%s", string(i.namespace), string(i.batchID), string(i.stampIndex))
}

// Namespace implements the storage.Item interface.
func (i Item) Namespace() string {
	return "stampIndex"
}

// Marshal implements the storage.Item interface.
func (i Item) Marshal() ([]byte, error) {
	switch {
	case len(i.namespace) == 0:
		return nil, errStampItemMarshalNamespaceInvalid
	case len(i.batchID) != swarm.HashSize:
		return nil, errStampItemMarshalBatchIDInvalid
	case len(i.stampIndex) != swarm.StampIndexSize:
		return nil, errStampItemMarshalBatchIndexInvalid
	}

	buf := make([]byte, 8+len(i.namespace)+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize+swarm.HashSize+1)

	l := 0
	binary.LittleEndian.PutUint64(buf[l:l+8], uint64(len(i.namespace)))
	l += 8
	copy(buf[l:l+len(i.namespace)], i.namespace)
	l += len(i.namespace)
	copy(buf[l:l+swarm.HashSize], i.batchID)
	l += swarm.HashSize
	copy(buf[l:l+swarm.StampIndexSize], i.stampIndex)
	l += swarm.StampIndexSize
	copy(buf[l:l+swarm.StampTimestampSize], i.StampTimestamp)
	l += swarm.StampTimestampSize
	copy(buf[l:l+swarm.HashSize], internal.AddressBytesOrZero(i.ChunkAddress))
	l += swarm.HashSize
	buf[l] = '0'
	if i.ChunkIsImmutable {
		buf[l] = '1'
	}
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (i *Item) Unmarshal(bytes []byte) error {
	if len(bytes) < 8 {
		return errStampItemUnmarshalInvalidSize
	}
	nsLen := int(binary.LittleEndian.Uint64(bytes))
	if len(bytes) != 8+nsLen+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize+swarm.HashSize+1 {
		return errStampItemUnmarshalInvalidSize
	}

	ni := new(Item)
	l := 8
	ni.namespace = append(make([]byte, 0, nsLen), bytes[l:l+nsLen]...)
	l += nsLen
	ni.batchID = append(make([]byte, 0, swarm.HashSize), bytes[l:l+swarm.HashSize]...)
	l += swarm.HashSize
	ni.stampIndex = append(make([]byte, 0, swarm.StampIndexSize), bytes[l:l+swarm.StampIndexSize]...)
	l += swarm.StampIndexSize
	ni.StampTimestamp = append(make([]byte, 0, swarm.StampTimestampSize), bytes[l:l+swarm.StampTimestampSize]...)
	l += swarm.StampTimestampSize
	ni.ChunkAddress = internal.AddressOrZero(bytes[l : l+swarm.HashSize])
	l += swarm.HashSize
	switch bytes[l] {
	case '0':
		ni.ChunkIsImmutable = false
	case '1':
		ni.ChunkIsImmutable = true
	default:
		return errStampItemUnmarshalChunkImmutableInvalid
	}
	*i = *ni
	return nil
}

// Clone  implements the storage.Item interface.
func (i *Item) Clone() storage.Item {
	if i == nil {
		return nil
	}
	return &Item{
		namespace:        append([]byte(nil), i.namespace...),
		batchID:          append([]byte(nil), i.batchID...),
		stampIndex:       append([]byte(nil), i.stampIndex...),
		StampTimestamp:   append([]byte(nil), i.StampTimestamp...),
		ChunkAddress:     i.ChunkAddress.Clone(),
		ChunkIsImmutable: i.ChunkIsImmutable,
	}
}

// String implements the fmt.Stringer interface.
func (i Item) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

// LoadOrStore tries to first load a stamp index related record from the store.
// If the record is not found, it will try to create and save a new record and
// return it.
func LoadOrStore(s storage.Store, namespace string, chunk swarm.Chunk) (item *Item, loaded bool, err error) {
	item, err = Load(s, namespace, chunk)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return &Item{
				namespace:        []byte(namespace),
				batchID:          chunk.Stamp().BatchID(),
				stampIndex:       chunk.Stamp().Index(),
				StampTimestamp:   chunk.Stamp().Timestamp(),
				ChunkAddress:     chunk.Address(),
				ChunkIsImmutable: chunk.Immutable(),
			}, false, Store(s, namespace, chunk)
		}
		return nil, false, err
	}
	return item, true, nil
}

// Load returns stamp index record related to the given namespace and chunk.
// The storage.ErrNotFound is returned if no record is found.
func Load(s storage.Store, namespace string, chunk swarm.Chunk) (*Item, error) {
	item := &Item{
		namespace:  []byte(namespace),
		batchID:    chunk.Stamp().BatchID(),
		stampIndex: chunk.Stamp().Index(),
	}
	err := s.Get(item)
	if err != nil {
		return nil, fmt.Errorf("failed to get stampindex.Item %s: %w", item, err)
	}
	return item, nil
}

// Store creates new or updated an existing stamp index
// record related to the given namespace and chunk.
func Store(s storage.Store, namespace string, chunk swarm.Chunk) error {
	item := &Item{
		namespace:        []byte(namespace),
		batchID:          chunk.Stamp().BatchID(),
		stampIndex:       chunk.Stamp().Index(),
		StampTimestamp:   chunk.Stamp().Timestamp(),
		ChunkAddress:     chunk.Address(),
		ChunkIsImmutable: chunk.Immutable(),
	}
	if err := s.Put(item); err != nil {
		return fmt.Errorf("failed to put stampindex.Item %s: %w", item, err)
	}
	return nil
}

// Delete removes the related stamp index record from the storage.
func Delete(s storage.Store, namespace string, chunk swarm.Chunk) error {
	item := &Item{
		namespace:  []byte(namespace),
		batchID:    chunk.Stamp().BatchID(),
		stampIndex: chunk.Stamp().Index(),
	}
	if err := s.Delete(item); err != nil {
		return fmt.Errorf("failed to delete stampindex.Item %s: %w", item, err)
	}
	return nil
}
