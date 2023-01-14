// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampindex

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// errStampItemMarshalBatchIndexInvalid is returned when trying
	// to marshal a stampIndexItem with invalid batchIndex.
	errStampItemMarshalBatchIndexInvalid = errors.New("marshal stampIndexItem: batchIndex is invalid")
	// errStampItemMarshalBatchIDInvalid is returned when trying to
	// marshal a stampIndexItem with invalid batchID.
	errStampItemMarshalBatchIDInvalid = errors.New("marshal stampIndexItem: batchID is invalid")
	// errStampItemMarshalBatchIDInvalid is returned when trying to
	// marshal a stampIndexItem with invalid namespace.
	errStampItemMarshalNamespaceInvalid = errors.New("marshal stampIndexItem: namespace is invalid")
	// errStampItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer with smaller size then is the size of the stampIndexItem fields.
	errStampItemUnmarshalInvalidSize = errors.New("unmarshal stampIndexItem: invalid size")
	// errStampItemUnmarshalChunkImmutableInvalid is returned when trying
	// to unmarshal buffer with invalid chunk immutable value.
	errStampItemUnmarshalChunkImmutableInvalid = errors.New("unmarshal stampIndexItem: chunk immutable is invalid")
)

var _ storage.Item = (*stampIndexItem)(nil)

// stampIndexItem is an store.Item that represents data relevant to stamp.
type stampIndexItem struct { // stampIndexItem
	namespace      []byte // The namespace of other related item.
	batchID        []byte
	batchIndex     []byte
	batchTimestamp []byte
	chunkImmutable bool
}

// ID implements the storage.Item interface.
func (i stampIndexItem) ID() string {
	return fmt.Sprintf("%s/%s/%s", string(i.namespace), string(i.batchID), string(i.batchIndex))
}

// Namespace implements the storage.Item interface.
func (i stampIndexItem) Namespace() string {
	return "stampIndex"
}

// Marshal implements the storage.Item interface.
func (i stampIndexItem) Marshal() ([]byte, error) {
	switch {
	case len(i.batchID) != swarm.HashSize:
		return nil, errStampItemMarshalBatchIDInvalid
	case len(i.batchIndex) != swarm.StampIndexSize:
		return nil, errStampItemMarshalBatchIndexInvalid
	case len(i.namespace) == 0:
		return nil, errStampItemMarshalNamespaceInvalid
	}

	var chImmutable byte = '0'
	if i.chunkImmutable {
		chImmutable = '1'
	}

	nsLen := len(i.namespace)
	buf := make([]byte, 8+nsLen+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize+1)

	binary.LittleEndian.PutUint64(buf, uint64(len(i.namespace)))
	copy(buf[8:], i.namespace)
	copy(buf[8+nsLen:], i.batchID)
	copy(buf[8+nsLen+swarm.HashSize:], i.batchIndex)
	copy(buf[8+nsLen+swarm.HashSize+swarm.StampIndexSize:], i.batchTimestamp)
	buf[8+nsLen+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize+1] = chImmutable
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (i *stampIndexItem) Unmarshal(bytes []byte) error {
	if len(bytes) < 8 {
		return errStampItemUnmarshalInvalidSize
	}
	nsLen := int(binary.LittleEndian.Uint64(bytes))
	if len(bytes) != 8+nsLen+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize+1 {
		return errStampItemUnmarshalInvalidSize
	}

	ni := new(stampIndexItem)
	ni.namespace = append(make([]byte, 0, nsLen), bytes[8:nsLen]...)
	ni.batchID = append(make([]byte, 0, swarm.HashSize), bytes[8+nsLen:8+nsLen+swarm.HashSize]...)
	ni.batchIndex = append(make([]byte, 0, swarm.StampIndexSize), bytes[8+nsLen+swarm.HashSize:8+nsLen+swarm.HashSize+swarm.StampIndexSize]...)
	ni.batchTimestamp = append(make([]byte, 0, swarm.StampTimestampSize), bytes[8+nsLen+swarm.HashSize+swarm.StampIndexSize:8+nsLen+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize]...)
	switch bytes[8+nsLen+swarm.HashSize+swarm.StampIndexSize+swarm.StampTimestampSize] {
	case '0':
		ni.chunkImmutable = false
	case '1':
		ni.chunkImmutable = true
	default:
		return errStampItemUnmarshalChunkImmutableInvalid
	}
	*i = *ni
	return nil
}

// String implements the fmt.Stringer interface.
func (i stampIndexItem) String() string {
	return path.Join(i.Namespace(), i.ID())
}

func LoadOrStore(s internal.Storage, namespace string, chunk swarm.Chunk) ([]byte, bool, error) {
	timestamp, immutable, err := Load(s, namespace, chunk)
	if errors.Is(err, storage.ErrNotFound) {
		return chunk.Stamp().Timestamp(), chunk.Immutable(), Store(s, namespace, chunk)
	}
	return timestamp, immutable, nil
}

func Load(s internal.Storage, namespace string, chunk swarm.Chunk) ([]byte, bool, error) {
	si := &stampIndexItem{
		namespace:  []byte(namespace),
		batchID:    chunk.Stamp().BatchID(),
		batchIndex: chunk.Stamp().Index(),
	}
	err := s.Store().Get(si)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get stampIndexItem %s: %w", si, err)
	}
	return si.batchTimestamp, si.chunkImmutable, nil
}

func Store(s internal.Storage, namespace string, chunk swarm.Chunk) error {
	ns := []byte(namespace)
	si := &stampIndexItem{
		namespace:      ns,
		batchID:        chunk.Stamp().BatchID(),
		batchIndex:     chunk.Stamp().Index(),
		batchTimestamp: chunk.Stamp().Timestamp(),
		chunkImmutable: chunk.Immutable(),
	}
	if err := s.Store().Put(si); err != nil {
		return fmt.Errorf("failed to put stampIndexItem %s: %w", si, err)
	}
	return nil
}

func Delete(s internal.Storage, namespace string, chunk swarm.Chunk) error {
	si := &stampIndexItem{
		namespace:  []byte(namespace),
		batchID:    chunk.Stamp().BatchID(),
		batchIndex: chunk.Stamp().Index(),
	}
	if err := s.Store().Delete(si); err != nil {
		return fmt.Errorf("failed to delete stampIndexItem %s: %w", si, err)
	}
	return nil
}
