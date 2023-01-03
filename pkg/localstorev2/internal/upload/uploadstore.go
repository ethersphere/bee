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
	"strconv"
	"time"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

// now returns the current time.Time; used in testing.
var now = time.Now

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

var (
	// errTagIDAddressItemMarshalAddressIsZero is returned when trying
	// to marshal a tagItem with an address that is zero.
	errTagItemMarshalAddressIsZero = errors.New("marshal tagItem: address is zero")

	// errTagIDAddressItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size tagItemSize.
	errTagItemUnmarshalInvalidSize = errors.New("unmarshal tagItem: invalid size")
)

// tagItemSize is the size of a marshaled tagItem.
const tagItemSize = swarm.HashSize + 8*8

var _ storage.Item = (*tagItem)(nil)

// tagItem is an store.Item that stores addresses of already seen chunks.
type tagItem struct {
	TagID     uint64        // unique identifier for the tag
	Total     uint64        // total no of chunks associated with this tag as calculated by user
	Split     uint64        // total no of chunks processed by the splitter for hashing
	Seen      uint64        // total no of chunks already seen
	Stored    uint64        // total no of chunks stored locally on the node
	Sent      uint64        // total no of chunks sent to the neighbourhood
	Synced    uint64        // total no of chunks synced with proof
	Address   swarm.Address // swarm.Address associated with this tag
	StartedAt int64         // start timestamp
}

// ID implements the storage.Item interface.
func (i tagItem) ID() string {
	return strconv.FormatUint(i.TagID, 10)
}

// Namespace implements the storage.Item interface.
func (i tagItem) Namespace() string {
	return "tagItem"
}

// Marshal implements the storage.Item interface.
func (i tagItem) Marshal() ([]byte, error) {
	buf := make([]byte, tagItemSize)
	binary.LittleEndian.PutUint64(buf, i.TagID)
	binary.LittleEndian.PutUint64(buf[8:], i.Total)
	binary.LittleEndian.PutUint64(buf[16:], i.Split)
	binary.LittleEndian.PutUint64(buf[24:], i.Seen)
	binary.LittleEndian.PutUint64(buf[32:], i.Stored)
	binary.LittleEndian.PutUint64(buf[40:], i.Sent)
	binary.LittleEndian.PutUint64(buf[48:], i.Synced)
	copy(buf[56:], internal.AddressBytesOrZero(i.Address))
	binary.LittleEndian.PutUint64(buf[56+swarm.HashSize:], uint64(i.StartedAt))
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size tagItemSize, an error is returned.
func (i *tagItem) Unmarshal(bytes []byte) error {
	if len(bytes) != tagItemSize {
		return errTagItemUnmarshalInvalidSize
	}
	ni := new(tagItem)
	ni.TagID = binary.LittleEndian.Uint64(bytes)
	ni.Total = binary.LittleEndian.Uint64(bytes[8:])
	ni.Split = binary.LittleEndian.Uint64(bytes[16:])
	ni.Seen = binary.LittleEndian.Uint64(bytes[24:])
	ni.Stored = binary.LittleEndian.Uint64(bytes[32:])
	ni.Sent = binary.LittleEndian.Uint64(bytes[40:])
	ni.Synced = binary.LittleEndian.Uint64(bytes[48:])
	ni.Address = internal.AddressOrZero(bytes[56 : 56+swarm.HashSize])
	ni.StartedAt = int64(binary.LittleEndian.Uint64(bytes[56+swarm.HashSize:]))
	*i = *ni
	return nil
}

// String implements the fmt.Stringer interface.
func (i tagItem) String() string {
	return path.Join(i.Namespace(), i.ID())
}

var (
	// errTagIDAddressItemMarshalAddressIsZero is returned when trying
	// to marshal a uploadItem with an address that is zero.
	errUploadItemMarshalAddressIsZero = errors.New("marshal uploadItem: address is zero")

	// errTagIDAddressItemUnmarshalInvalidSize is returned when trying
	// to unmarshal buffer that is not of size uploadItemSize.
	errUploadItemUnmarshalInvalidSize = errors.New("unmarshal uploadItem: invalid size")
)

// uploadItemSize is the size of a marshaled uploadItem.
const uploadItemSize = swarm.HashSize + 8

var _ storage.Item = (*uploadItem)(nil)

// uploadItem is an store.Item that stores addresses of already seen chunks.
type uploadItem struct {
	Address swarm.Address
	TagID   uint64
	Synced  int64
}

// ID implements the storage.Item interface.
func (i uploadItem) ID() string {
	return i.Address.ByteString()
}

// Namespace implements the storage.Item interface.
func (i uploadItem) Namespace() string {
	return "UploadItem"
}

// Marshal implements the storage.Item interface.
// If the Address is zero, an error is returned.
func (i uploadItem) Marshal() ([]byte, error) {
	if i.Address.IsZero() {
		return nil, errUploadItemMarshalAddressIsZero
	}
	buf := make([]byte, uploadItemSize)
	copy(buf, i.Address.Bytes())
	binary.LittleEndian.PutUint64(buf[swarm.HashSize:], i.TagID)
	binary.LittleEndian.PutUint64(buf[swarm.HashSize+8:], uint64(i.Synced))
	return buf, nil
}

// Unmarshal implements the storage.Item interface.
// If the buffer is not of size pushItemSize, an error is returned.
func (i *uploadItem) Unmarshal(bytes []byte) error {
	if len(bytes) != uploadItemSize {
		return errUploadItemUnmarshalInvalidSize
	}
	ni := new(uploadItem)
	ni.Address = internal.AddressOrZero(bytes[:swarm.HashSize])
	ni.TagID = binary.LittleEndian.Uint64(bytes[swarm.HashSize:])
	ni.Synced = int64(binary.LittleEndian.Uint64(bytes[swarm.HashSize+8:]))
	*i = *ni
	return nil
}

// String implements the fmt.Stringer interface.
func (i uploadItem) String() string {
	return path.Join(i.Namespace(), i.ID())
}

// ChunkPutter returns a storage.Putter which will store the given chunk.
func ChunkPutter(s internal.Storage, tagID uint64) (storage.Putter, error) {
	return storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
		ui := &uploadItem{
			Address: chunk.Address(),
			TagID:   tagID,
		}
		switch exists, err := s.Store().Has(ui); {
		case err != nil:
			return fmt.Errorf("store has item %q call failed: %w", ui, err)
		case exists:
			return nil
		}
		if err := s.Store().Put(tai); err != nil {
			return fmt.Errorf("store put item %q call failed: %w", tai, err)
		}

		pi := &pushItem{
			Timestamp: now().Unix(),
			Address:   chunk.Address(),
			TagID:     tagID,
		}
		if err := s.Store().Put(pi); err != nil {
			return fmt.Errorf("store put item %q call failed: %w", pi, err)
		}
		err := s.ChunkStore().Put(ctx, chunk)
		if err != nil {
			return fmt.Errorf("chunk store put chunk %q call failed: %w", chunk.Address(), err)
		}
		return nil
	}), nil
}
