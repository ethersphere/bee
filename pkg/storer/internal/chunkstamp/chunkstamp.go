// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstamp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	// errMarshalInvalidChunkStampItemScope is returned during marshaling if the scope is not set.
	errMarshalInvalidChunkStampItemScope = errors.New("marshal chunkstamp.Item: invalid scope")
	// errMarshalInvalidChunkStampAddress is returned during marshaling if the address is zero.
	errMarshalInvalidChunkStampItemAddress = errors.New("marshal chunkstamp.item: invalid address")
	// errUnmarshalInvalidChunkStampAddress is returned during unmarshaling if the address is not set.
	errUnmarshalInvalidChunkStampItemAddress = errors.New("unmarshal chunkstamp.item: invalid address")
	// errMarshalInvalidChunkStamp is returned if the stamp is invalid during marshaling.
	errMarshalInvalidChunkStampItemStamp = errors.New("marshal chunkstamp.item: invalid stamp")
	// errUnmarshalInvalidChunkStampSize is returned during unmarshaling if the passed buffer is not the expected size.
	errUnmarshalInvalidChunkStampItemSize = errors.New("unmarshal chunkstamp.item: invalid size")
)

var _ storage.Item = (*Item)(nil)

// Item is the index used to represent a stamp for a chunk.
//
// Going ahead we will support multiple stamps on chunks. This Item will allow
// mapping multiple stamps to a single address. For this reason, the address is
// part of the Namespace and can be used to iterate on all the stamps for this
// address.
type Item struct {
	scope   []byte // The scope of other related item.
	address swarm.Address
	stamp   swarm.Stamp
}

// ID implements the storage.Item interface.
func (i *Item) ID() string {
	return storageutil.JoinFields(string(i.stamp.BatchID()), string(i.stamp.Index()))
}

// Namespace implements the storage.Item interface.
func (i *Item) Namespace() string {
	return storageutil.JoinFields("chunkStamp", string(i.scope), i.address.ByteString())
}

func (i *Item) SetScope(ns []byte) {
	i.scope = ns
}

// Marshal implements the storage.Item interface.
// address is not part of the payload which is stored, as address is part of the
// prefix, hence already known before querying this object. This will be reused
// during unmarshaling.
func (i *Item) Marshal() ([]byte, error) {
	// The address is not part of the payload, but it is used to create the
	// scope so it is better if we check that the address is correctly
	// set here before it is stored in the underlying storage.

	switch {
	case len(i.scope) == 0:
		return nil, errMarshalInvalidChunkStampItemScope
	case i.address.IsZero():
		return nil, errMarshalInvalidChunkStampItemAddress
	case i.stamp == nil:
		return nil, errMarshalInvalidChunkStampItemStamp
	}

	buf := make([]byte, 8+len(i.scope)+postage.StampSize)

	l := 0
	binary.LittleEndian.PutUint64(buf[l:l+8], uint64(len(i.scope)))
	l += 8
	copy(buf[l:l+len(i.scope)], i.scope)
	l += len(i.scope)
	data, err := i.stamp.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chunkstamp.item: %w", err)
	}
	copy(buf[l:l+postage.StampSize], data)

	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (i *Item) Unmarshal(bytes []byte) error {
	if len(bytes) < 8 {
		return errUnmarshalInvalidChunkStampItemSize
	}
	nsLen := int(binary.LittleEndian.Uint64(bytes))
	if len(bytes) != 8+nsLen+postage.StampSize {
		return errUnmarshalInvalidChunkStampItemSize
	}

	// Ensure that the address is set already in the item.
	if i.address.IsZero() {
		return errUnmarshalInvalidChunkStampItemAddress
	}

	ni := &Item{address: i.address.Clone()}
	l := 8
	ni.scope = append(make([]byte, 0, nsLen), bytes[l:l+nsLen]...)
	l += nsLen
	stamp := new(postage.Stamp)
	if err := stamp.UnmarshalBinary(bytes[l : l+postage.StampSize]); err != nil {
		if errors.Is(err, postage.ErrStampInvalid) {
			return errUnmarshalInvalidChunkStampItemSize
		}
		return fmt.Errorf("unable to unmarshal chunkstamp.item: %w", err)
	}
	ni.stamp = stamp
	*i = *ni
	return nil
}

// Clone implements the storage.Item interface.
func (i *Item) Clone() storage.Item {
	if i == nil {
		return nil
	}
	clone := &Item{
		scope:   append([]byte(nil), i.scope...),
		address: i.address.Clone(),
	}
	if i.stamp != nil {
		clone.stamp = i.stamp.Clone()
	}
	return clone
}

// String implements the storage.Item interface.
func (i Item) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

// Load returns first found swarm.Stamp related to the given address.
func Load(s storage.Reader, scope string, addr swarm.Address) (swarm.Stamp, error) {
	return LoadWithBatchID(s, scope, addr, nil)
}

// LoadWithBatchID returns swarm.Stamp related to the given address and batchID.
func LoadWithBatchID(s storage.Reader, scope string, addr swarm.Address, batchID []byte) (swarm.Stamp, error) {
	var stamp swarm.Stamp

	found := false
	err := s.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return &Item{
					scope:   []byte(scope),
					address: addr,
				}
			},
		},
		func(res storage.Result) (bool, error) {
			item := res.Entry.(*Item)
			if batchID == nil || bytes.Equal(batchID, item.stamp.BatchID()) {
				stamp = item.stamp
				found = true
				return true, nil
			}
			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("stamp not found for batchID %x: %w", batchID, storage.ErrNotFound)
	}

	return stamp, nil
}

// LoadWithStampHash returns swarm.Stamp related to the given address and stamphash.
func LoadWithStampHash(s storage.Reader, scope string, addr swarm.Address, hash []byte) (swarm.Stamp, error) {
	var stamp swarm.Stamp

	found := false
	err := s.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return &Item{
					scope:   []byte(scope),
					address: addr,
				}
			},
		},
		func(res storage.Result) (bool, error) {
			item := res.Entry.(*Item)
			h, err := item.stamp.Hash()
			if err != nil {
				return false, err
			}
			if bytes.Equal(hash, h) {
				stamp = item.stamp
				found = true
				return true, nil
			}
			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("stamp not found for hash %x: %w", hash, storage.ErrNotFound)
	}

	return stamp, nil
}

// Store creates new or updated an existing stamp index
// record related to the given scope and chunk.
func Store(s storage.IndexStore, scope string, chunk swarm.Chunk) error {
	item := &Item{
		scope:   []byte(scope),
		address: chunk.Address(),
		stamp:   chunk.Stamp(),
	}
	if err := s.Put(item); err != nil {
		return fmt.Errorf("unable to put chunkstamp.item %s: %w", item, err)
	}
	return nil
}

// DeleteAll removes all swarm.Stamp related to the given address.
func DeleteAll(s storage.IndexStore, scope string, addr swarm.Address) error {
	var stamps []swarm.Stamp
	err := s.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return &Item{
					scope:   []byte(scope),
					address: addr,
				}
			},
		},
		func(res storage.Result) (bool, error) {
			stamps = append(stamps, res.Entry.(*Item).stamp)
			return false, nil
		},
	)
	if err != nil {
		return err
	}

	var errs error
	for _, stamp := range stamps {
		errs = errors.Join(
			errs,
			s.Delete(&Item{
				scope:   []byte(scope),
				address: addr,
				stamp:   stamp,
			}),
		)
	}
	return errs
}

// Delete removes a stamp associated with an chunk and batchID.
func Delete(s storage.IndexStore, scope string, addr swarm.Address, batchId []byte) error {
	stamp, err := LoadWithBatchID(s, scope, addr, batchId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return err
	}
	return s.Delete(&Item{
		scope:   []byte(scope),
		address: addr,
		stamp:   stamp,
	})
}

func DeleteWithStamp(
	writer storage.Writer,
	scope string,
	addr swarm.Address,
	stamp swarm.Stamp,
) error {
	return writer.Delete(&Item{
		scope:   []byte(scope),
		address: addr,
		stamp:   stamp,
	})
}
