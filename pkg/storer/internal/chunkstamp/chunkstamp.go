// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstamp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/postage"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// errMarshalInvalidChunkStampItemNamespace is returned during marshaling if the namespace is not set.
	errMarshalInvalidChunkStampItemNamespace = errors.New("marshal chunkstamp.Item: invalid namespace")
	// errMarshalInvalidChunkStampAddress is returned during marshaling if the address is zero.
	errMarshalInvalidChunkStampItemAddress = errors.New("marshal chunkstamp.item: invalid address")
	// errUnmarshalInvalidChunkStampAddress is returned during unmarshaling if the address is not set.
	errUnmarshalInvalidChunkStampItemAddress = errors.New("unmarshal chunkstamp.item: invalid address")
	// errMarshalInvalidChunkStamp is returned if the stamp is invalid during marshaling.
	errMarshalInvalidChunkStampItemStamp = errors.New("marshal chunkstamp.item: invalid stamp")
	// errUnmarshalInvalidChunkStampSize is returned during unmarshaling if the passed buffer is not the expected size.
	errUnmarshalInvalidChunkStampItemSize = errors.New("unmarshal chunkstamp.item: invalid size")
)

var _ storage.Item = (*item)(nil)

// item is the index used to represent a stamp for a chunk.
//
// Going ahead we will support multiple stamps on chunks. This item will allow
// mapping multiple stamps to a single address. For this reason, the address is
// part of the Namespace and can be used to iterate on all the stamps for this
// address.
type item struct {
	namespace []byte // The namespace of other related item.
	address   swarm.Address
	stamp     swarm.Stamp
}

// ID implements the storage.Item interface.
func (i *item) ID() string {
	return storageutil.JoinFields(string(i.stamp.BatchID()), string(i.stamp.Index()))
}

// Namespace implements the storage.Item interface.
func (i *item) Namespace() string {
	return storageutil.JoinFields("chunkStamp", string(i.namespace), i.address.ByteString())
}

// Marshal implements the storage.Item interface.
// address is not part of the payload which is stored, as address is part of the
// prefix, hence already known before querying this object. This will be reused
// during unmarshaling.
func (i *item) Marshal() ([]byte, error) {
	// The address is not part of the payload, but it is used to create the
	// namespace so it is better if we check that the address is correctly
	// set here before it is stored in the underlying storage.

	switch {
	case len(i.namespace) == 0:
		return nil, errMarshalInvalidChunkStampItemNamespace
	case i.address.IsZero():
		return nil, errMarshalInvalidChunkStampItemAddress
	case i.stamp == nil:
		return nil, errMarshalInvalidChunkStampItemStamp
	}

	buf := make([]byte, 8+len(i.namespace)+postage.StampSize)

	l := 0
	binary.LittleEndian.PutUint64(buf[l:l+8], uint64(len(i.namespace)))
	l += 8
	copy(buf[l:l+len(i.namespace)], i.namespace)
	l += len(i.namespace)
	data, err := i.stamp.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chunkstamp.item: %w", err)
	}
	copy(buf[l:l+postage.StampSize], data)

	return buf, nil
}

// Unmarshal implements the storage.Item interface.
func (i *item) Unmarshal(bytes []byte) error {
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

	ni := &item{address: i.address.Clone()}
	l := 8
	ni.namespace = append(make([]byte, 0, nsLen), bytes[l:l+nsLen]...)
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
func (i *item) Clone() storage.Item {
	if i == nil {
		return nil
	}
	clone := &item{
		namespace: append([]byte(nil), i.namespace...),
		address:   i.address.Clone(),
	}
	if i.stamp != nil {
		clone.stamp = i.stamp.Clone()
	}
	return clone
}

// String implements the storage.Item interface.
func (i item) String() string {
	return storageutil.JoinFields(i.Namespace(), i.ID())
}

// Load returns first found swarm.Stamp related to the given address.
func Load(s storage.Reader, namespace string, addr swarm.Address) (swarm.Stamp, error) {
	return LoadWithBatchID(s, namespace, addr, nil)
}

// LoadWithBatchID returns swarm.Stamp related to the given address and batchID.
func LoadWithBatchID(s storage.Reader, namespace string, addr swarm.Address, batchID []byte) (swarm.Stamp, error) {
	var stamp swarm.Stamp

	found := false
	err := s.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return &item{
					namespace: []byte(namespace),
					address:   addr,
				}
			},
		},
		func(res storage.Result) (bool, error) {
			item := res.Entry.(*item)
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

// Store creates new or updated an existing stamp index
// record related to the given namespace and chunk.
func Store(s storage.Writer, namespace string, chunk swarm.Chunk) error {
	item := &item{
		namespace: []byte(namespace),
		address:   chunk.Address(),
		stamp:     chunk.Stamp(),
	}
	if err := s.Put(item); err != nil {
		return fmt.Errorf("unable to put chunkstamp.item %s: %w", item, err)
	}
	return nil
}

// DeleteAll removes all swarm.Stamp related to the given address.
func DeleteAll(s storage.Store, namespace string, addr swarm.Address) error {
	var stamps []swarm.Stamp
	err := s.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return &item{
					namespace: []byte(namespace),
					address:   addr,
				}
			},
		},
		func(res storage.Result) (bool, error) {
			stamps = append(stamps, res.Entry.(*item).stamp)
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
			s.Delete(&item{
				namespace: []byte(namespace),
				address:   addr,
				stamp:     stamp,
			}),
		)
	}
	return errs
}

// Delete removes a stamp associated with an chunk and batchID.
func Delete(s storage.Store, batch storage.Writer, namespace string, addr swarm.Address, batchId []byte) error {
	stamp, err := LoadWithBatchID(s, namespace, addr, batchId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return err
	}
	return batch.Delete(&item{
		namespace: []byte(namespace),
		address:   addr,
		stamp:     stamp,
	})
}

func DeleteWithStamp(
	writer storage.Writer,
	namespace string,
	addr swarm.Address,
	stamp swarm.Stamp,
) error {
	return writer.Delete(&item{
		namespace: []byte(namespace),
		address:   addr,
		stamp:     stamp,
	})
}
