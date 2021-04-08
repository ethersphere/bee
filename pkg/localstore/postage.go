// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"errors"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	// ErrBatchOverissued is returned if number of chunks found in neighbourhood extrapolates to overissued stamp
	// count(batch, po) > 1<< (depth(batch) - po)
	ErrBatchOverissued = errors.New("postage batch overissued")
)

type postageBatches struct {
	// postage batch to chunks index
	chunks shed.Index
	po     func(swarm.Address) (bin uint8)
	db     *DB
}

func newPostageBatches(db *DB) (*postageBatches, error) {
	// po applied to the item address returns the proximity order (as int)
	// of the chunk relative to the node base address
	// return value is max swarm.MaxPO
	pof := db.po

	chunksIndex, err := db.shed.NewIndex("BatchID|PO|Hash->nil", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 65)
			copy(key[:32], fields.BatchID)
			key[32] = pof(swarm.NewAddress(fields.Address))
			copy(key[33:], fields.Address)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BatchID = key[:32]
			e.Address = key[33:65]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			return nil, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	return &postageBatches{
		chunks: chunksIndex,
		po:     pof,
		db:     db,
	}, nil
}

func (p *postageBatches) putInBatch(batch *leveldb.Batch, item shed.Item) error {
	err := p.chunks.PutInBatch(batch, item)
	if err != nil {
		return err
	}
	return nil
}

func (p *postageBatches) deleteInBatch(batch *leveldb.Batch, item shed.Item) error {
	err := p.chunks.DeleteInBatch(batch, item)
	if err != nil {
		return err
	}
	return nil
}

// UnreserveBatch atomically unpins chunks of a batch in proximity order upto and including po
// and marks the batch pinned within radius po
// if batch is marked as pinned within radius r>po, then do nothing
// unpinning will result in all chunks  with pincounter 0 to be put in the gc index
// so if a chunk was only pinned by the reserve, unreserving it  will make it gc-able
func (db *DB) UnreserveBatch(id []byte, oldRadius, newRadius uint8) error {
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	// todo add metrics

	batch := new(leveldb.Batch)
	var gcSizeChange int64 // number to add or subtract from gcSize
	unpin := func(item shed.Item) (stop bool, err error) {
		c, err := db.setUnpin(batch, swarm.NewAddress(item.Address))
		if err != nil {
			return false, err
		}

		// if the batch is unreserved we should remove the chunk
		// from the pull index
		item2, err := db.retrievalDataIndex.Get(item)
		if err != nil {
			return false, err
		}
		err = db.pullIndex.DeleteInBatch(batch, item2)
		if err != nil {
			return false, err
		}

		gcSizeChange += c
		return false, err
	}

	// iterate over chunk in bins
	// TODO the initial value needs to change to the previous
	// batch radius value.
	for bin := oldRadius; bin < newRadius; bin++ {
		err := db.postage.chunks.Iterate(unpin, &shed.IterateOptions{Prefix: append(id, bin)})
		if err != nil {
			return err
		}
		// adjust gcSize
		if err := db.incGCSizeInBatch(batch, gcSizeChange); err != nil {
			return err
		}
		if err := db.shed.WriteBatch(batch); err != nil {
			return err
		}
		batch = new(leveldb.Batch)
		gcSizeChange = 0
	}
	return nil
}

func (p *postageBatches) withinRadius(item shed.Item) bool {
	po := p.po(swarm.NewAddress(item.Address))

	return po >= item.Radius
}
