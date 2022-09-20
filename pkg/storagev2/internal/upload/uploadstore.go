// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
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
	return i.Address.ByteString()
}

// Namespace implements the storage.Item interface.
func (i tagIDAddressItem) Namespace() string {
	return fmt.Sprintf("TagIDAddressItem/%d", i.TagID)
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

// String implements the fmt.Stringer interface.
func (i tagIDAddressItem) String() string {
	return path.Join(i.Namespace(), i.ID())
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
	Timestamp int64
	Address   swarm.Address
	TagID     uint64
}

// ID implements the storage.Item interface.
func (i pushItem) ID() string {
	return fmt.Sprintf("%d/%s", i.Timestamp, i.Address.ByteString())
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
	binary.LittleEndian.PutUint64(buf, uint64(i.Timestamp))
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
	ni.Timestamp = int64(binary.LittleEndian.Uint64(bytes))
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), bytes[8:8+swarm.HashSize]...))
	ni.TagID = binary.LittleEndian.Uint64(bytes[8+swarm.HashSize:])
	*i = *ni
	return nil
}

// String implements the fmt.Stringer interface.
func (i pushItem) String() string {
	return path.Join(i.Namespace(), i.ID())
}

// ChunkPutter returns a storage.Putter which will store the given chunk.
func ChunkPutter(s internal.Storage, tag uint64) (storage.Putter, error) {
	return storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) (bool, error) {
		tai := &tagIDAddressItem{
			Address: chunk.Address(),
			TagID:   tag,
		}
		switch exists, err := s.Store().Has(tai); {
		case err != nil:
			return false, fmt.Errorf("storage has item %q call failed: %w", tai, err)
		case exists:
			return true, nil
		}
		if err := s.Store().Put(tai); err != nil {
			return false, fmt.Errorf("storage put item %q call failed: %w", tai, err)
		}

		pi := &pushItem{
			Timestamp: now().Unix(),
			Address:   chunk.Address(),
			TagID:     tag,
		}
		if err := s.Store().Put(pi); err != nil {
			return false, fmt.Errorf("storage put item %q call failed: %w", pi, err)
		}
		exists, err := s.ChunkStore().Put(ctx, chunk)
		if err != nil {
			return false, fmt.Errorf("chunk store put chunk %q call failed: %w", chunk.Address(), err)
		}
		return exists, nil
	}), nil
}
