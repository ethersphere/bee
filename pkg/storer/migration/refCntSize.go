// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const oldRretrievalIndexItemSize = swarm.HashSize + 8 + sharky.LocationSize + 1

var _ storage.Item = (*OldRetrievalIndexItem)(nil)

var (
	// errMarshalInvalidRetrievalIndexAddress is returned if the RetrievalIndexItem address is zero during marshaling.
	errMarshalInvalidRetrievalIndexAddress = errors.New("marshal RetrievalIndexItem: address is zero")
	// errMarshalInvalidRetrievalIndexLocation is returned if the RetrievalIndexItem location is invalid during marshaling.
	errMarshalInvalidRetrievalIndexLocation = errors.New("marshal RetrievalIndexItem: location is invalid")
	// errUnmarshalInvalidRetrievalIndexSize is returned during unmarshaling if the passed buffer is not the expected size.
	errUnmarshalInvalidRetrievalIndexSize = errors.New("unmarshal RetrievalIndexItem: invalid size")
	// errUnmarshalInvalidRetrievalIndexLocationBytes is returned during unmarshaling if the location buffer is invalid.
	errUnmarshalInvalidRetrievalIndexLocationBytes = errors.New("unmarshal RetrievalIndexItem: invalid location bytes")
)

// OldRetrievalIndexItem is the index which gives us the sharky location from the swarm.Address.
// The RefCnt stores the reference of each time a Put operation is issued on this Address.
type OldRetrievalIndexItem struct {
	Address   swarm.Address
	Timestamp uint64
	Location  sharky.Location
	RefCnt    uint8
}

func (r *OldRetrievalIndexItem) ID() string { return r.Address.ByteString() }

func (OldRetrievalIndexItem) Namespace() string { return "retrievalIdx" }

// Stored in bytes as:
// |--Address(32)--|--Timestamp(8)--|--Location(7)--|--RefCnt(1)--|
func (r *OldRetrievalIndexItem) Marshal() ([]byte, error) {
	if r.Address.IsZero() {
		return nil, errMarshalInvalidRetrievalIndexAddress
	}

	locBuf, err := r.Location.MarshalBinary()
	if err != nil {
		return nil, errMarshalInvalidRetrievalIndexLocation
	}

	buf := make([]byte, oldRretrievalIndexItemSize)
	copy(buf, r.Address.Bytes())
	binary.LittleEndian.PutUint64(buf[swarm.HashSize:], r.Timestamp)
	copy(buf[swarm.HashSize+8:], locBuf)
	buf[oldRretrievalIndexItemSize-1] = r.RefCnt
	return buf, nil
}

func (r *OldRetrievalIndexItem) Unmarshal(buf []byte) error {
	if len(buf) != oldRretrievalIndexItemSize {
		return errUnmarshalInvalidRetrievalIndexSize
	}

	loc := new(sharky.Location)
	if err := loc.UnmarshalBinary(buf[swarm.HashSize+8:]); err != nil {
		return errUnmarshalInvalidRetrievalIndexLocationBytes
	}

	ni := new(OldRetrievalIndexItem)
	ni.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	ni.Timestamp = binary.LittleEndian.Uint64(buf[swarm.HashSize:])
	ni.Location = *loc
	ni.RefCnt = buf[oldRretrievalIndexItemSize-1]
	*r = *ni
	return nil
}

func (r *OldRetrievalIndexItem) Clone() storage.Item {
	if r == nil {
		return nil
	}
	return &OldRetrievalIndexItem{
		Address:   r.Address.Clone(),
		Timestamp: r.Timestamp,
		Location:  r.Location,
		RefCnt:    r.RefCnt,
	}
}

func (r OldRetrievalIndexItem) String() string {
	return storageutil.JoinFields(r.Namespace(), r.ID())
}

func RefCountSizeInc(s storage.BatchStore, logger log.Logger) func() error {
	return func() error {

		logger := logger.WithName("migration-RefCountSizeInc").Register()

		logger.Info("starting migration of replacing chunkstore items to increase refCnt capacity")

		var itemsToDelete []*OldRetrievalIndexItem

		err := s.Iterate(
			storage.Query{
				Factory: func() storage.Item { return &OldRetrievalIndexItem{} },
			},
			func(res storage.Result) (bool, error) {
				item := res.Entry.(*OldRetrievalIndexItem)
				itemsToDelete = append(itemsToDelete, item)
				return false, nil
			},
		)
		if err != nil {
			return err
		}

		for i := 0; i < len(itemsToDelete); i += 10000 {
			end := i + 10000
			if end > len(itemsToDelete) {
				end = len(itemsToDelete)
			}

			b := s.Batch(context.Background())
			for _, item := range itemsToDelete[i:end] {

				//create new
				err = b.Put(&chunkstore.RetrievalIndexItem{
					Address:   item.Address,
					Timestamp: item.Timestamp,
					Location:  item.Location,
					RefCnt:    uint32(item.RefCnt),
				})
				if err != nil {
					return err
				}
			}

			err = b.Commit()
			if err != nil {
				return err
			}
		}

		logger.Info("migration complete")

		return nil
	}
}
