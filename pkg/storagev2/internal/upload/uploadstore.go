// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/internal"
	"github.com/ethersphere/bee/pkg/swarm"
)

// now returns the current time.Time; used in testing.
var now = time.Now

var (
	// errTagIDAddressItemMarshalAddressIsZero is returned when trying
	// to marshal a tagIDAddressItem with an address that is zero.
	errTagIDAddressItemMarshalAddressIsZero = errors.New("marshal tagIDAddressItem: address is zero")

	// errTagIDAddressItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size tagIDAddressItemSize.
	errTagIDAddressItemUnmarshalInvalidSize = errors.New("unmarshal tagIDAddressItem: invalid size")
)

// tagIDAddressItemSize is the size of a marshaled tagIDAddressItem.
const tagIDAddressItemSize = swarm.HashSize + 8

var _ storage.Item = (*tagIDAddressItem)(nil)

// tagIDAddressItem is an store.Item that stores addresses of already seen chunks.
type tagIDAddressItem struct {
	TagID   uint64
	Address swarm.Address
}

// ID implements the storage.Item interface.
func (i tagIDAddressItem) ID() string {
	return fmt.Sprintf("%d_%s", i.TagID, i.Address.ByteString())
}

// Namespace implements the storage.Item interface.
func (i tagIDAddressItem) Namespace() string {
	return "TagIDAddressItem"
}

// Marshal implements the storage.Item interface.
// If the Address is zero, an error is returned.
func (i tagIDAddressItem) Marshal() ([]byte, error) {
	if i.Address.IsZero() {
		return nil, errTagIDAddressItemMarshalAddressIsZero
	}
	buf := make([]byte, tagIDAddressItemSize)
	binary.LittleEndian.PutUint64(buf, i.TagID)
	copy(buf[8:], i.Address.Bytes())
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size pushItemSize, an error is returned.
func (i *tagIDAddressItem) Unmarshal(bytes []byte) error {
	if len(bytes) != tagIDAddressItemSize {
		return errTagIDAddressItemUnmarshalInvalidSize
	}
	ni := new(tagIDAddressItem)
	ni.TagID = binary.LittleEndian.Uint64(bytes)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), bytes[8:]...))
	*i = *ni
	return nil
}

var (
	// errPushItemMarshalAddressIsZero is returned when trying
	// to marshal a pushItem with an address that is zero.
	errPushItemMarshalAddressIsZero = errors.New("marshal pushItem: address is zero")

	// errPushItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size pushItemSize.
	errPushItemUnmarshalInvalidSize = errors.New("unmarshal pushItem: invalid size")
)

// pushItemSize is the size of a marshaled pushItem.
const pushItemSize = 8 + swarm.HashSize + 8

var _ storage.Item = (*pushItem)(nil)

// pushItem is an store.Item that represents data relevant to push.
// The key is a combination of Timestamp and Address, where the
// Timestamp provides an order to iterate.
type pushItem struct {
	Timestamp uint64
	Address   swarm.Address
	TagID     uint64
}

// ID implements the storage.Item interface.
func (i pushItem) ID() string {
	return fmt.Sprintf("%d_%s", i.Timestamp, i.Address.ByteString())
}

// Namespace implements the storage.Item interface.
func (i pushItem) Namespace() string {
	return "pushIndex"
}

// Marshal implements the storage.Item interface.
// If the Address is zero, an error is returned.
func (i pushItem) Marshal() ([]byte, error) {
	if i.Address.IsZero() {
		return nil, errPushItemMarshalAddressIsZero
	}
	buf := make([]byte, pushItemSize)
	binary.LittleEndian.PutUint64(buf, i.Timestamp)
	copy(buf[8:], i.Address.Bytes())
	binary.LittleEndian.PutUint64(buf[8+swarm.HashSize:], i.TagID)
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size pushItemSize, an error is returned.
func (i *pushItem) Unmarshal(bytes []byte) error {
	if len(bytes) != pushItemSize {
		return errPushItemUnmarshalInvalidSize
	}
	ni := new(pushItem)
	ni.Timestamp = binary.LittleEndian.Uint64(bytes)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), bytes[8:8+swarm.HashSize]...))
	ni.TagID = binary.LittleEndian.Uint64(bytes[8+swarm.HashSize:])
	*i = *ni
	return nil
}

// ChunkPutter returns a storage.Putter which will store the given chunk.
func ChunkPutter(s internal.Storage, tag uint64) (storage.Putter, error) {
	return storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) (bool, error) {
		tai := &tagIDAddressItem{
			Address: chunk.Address(),
			TagID:   tag,
		}
		switch exists, err := s.Storage().Has(tai); {
		case err != nil:
			return false, err
		case exists:
			return true, nil
		}
		if err := s.Storage().Put(tai); err != nil {
			return false, err
		}

		pi := &pushItem{
			Timestamp: uint64(now().Unix()),
			Address:   chunk.Address(),
			TagID:     tag,
		}
		if err := s.Storage().Put(pi); err != nil {
			return false, err
		}
		return s.ChunkStore().Put(ctx, chunk)
	}), nil
}
