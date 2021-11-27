package localstore

import (
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
)

// recovery tries to recover a dirty database.
// it returns a slice of the _taken_ locations in
// sharky according to the items it sees in the retrieval
// data index.
func recovery(db *DB) ([]sharky.Location, error) {
	// - go through all retrieval data index entries
	// - find all used locations in sharky
	// - return them so that sharky can be initialized with them

	// first define the index instance
	headerSize := 16 + postage.StampSize

	retrievalDataIndex, err := db.shed.NewIndex("Address->StoreTimestamp|BinID|BatchID|BatchIndex|Sig|Location", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, headerSize)
			binary.BigEndian.PutUint64(b[:8], fields.BinID)
			binary.BigEndian.PutUint64(b[8:16], uint64(fields.StoreTimestamp))
			stamp, err := postage.NewStamp(fields.BatchID, fields.Index, fields.Timestamp, fields.Sig).MarshalBinary()
			if err != nil {
				return nil, err
			}
			copy(b[16:], stamp)
			value = append(b, fields.Location...)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.StoreTimestamp = int64(binary.BigEndian.Uint64(value[8:16]))
			e.BinID = binary.BigEndian.Uint64(value[:8])
			stamp := new(postage.Stamp)
			if err = stamp.UnmarshalBinary(value[16:headerSize]); err != nil {
				return e, err
			}
			e.BatchID = stamp.BatchID()
			e.Index = stamp.Index()
			e.Timestamp = stamp.Timestamp()
			e.Sig = stamp.Sig()
			e.Location = value[headerSize:]
			return e, nil
		},
	})

	// gc size
	gcSize, err := db.shed.NewUint64Field("gc-size")
	if err != nil {
		return nil, err
	}

	// reserve size
	reserveSize, err := db.shed.NewUint64Field("reserve-size")
	if err != nil {
		return nil, err
	}

	vr, err := reserveSize.Get()
	if err != nil {
		return nil, err
	}
	vc, err := gcSize.Get()
	if err != nil {
		return nil, err
	}

	// this operation is memory intensive. we will preallocate the
	// minimum size of locations we expect to see. this way we can keep
	// the slice reallocations to a minimum.
	usedLocations := make([]sharky.Location, 0, vr+vc)

	retrievalDataIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		loc, err := sharky.LocationFromBinary(item.Location)
		if err != nil {
			return false, fmt.Errorf("location from binary: %w", err)
		}
		usedLocations = append(usedLocations, *loc)
		return false, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return usedLocations, nil
}
