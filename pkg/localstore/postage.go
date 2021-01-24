// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"errors"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	// ErrBatchOverissued is returned if number of chunks found in neighbourhood extrapolates to overissued stamp
	// count(batch, po) > 1<< (depth(batch) - po)
	ErrBatchOverissued = errors.New("postage batch overissued")
	// ErrBatchNotFound is returned when the postage batch is not found or expired
	ErrBatchNotFound = errors.New("postage batch not found or expired")
)

// BatchStore interface should come from postage pkg
type BatchStore interface {
	Get(id []byte) (*postage.Batch, error)
	Reserve
}

type postageBatches struct {
	// postage batch to chunks index
	chunks     shed.Index
	counts     shed.Index
	po         func(itemAddr []byte) (bin int)
	batchStore BatchStore
	db         *DB
}

func newPostageBatches(db *DB, batchStore BatchStore) (*postageBatches, error) {
	// po applied to the item address returns the proximity order (as int)
	// of the chunk relative to the node base address
	// return value is max swarm.MaxPO
	pof := func(addr []byte) int {
		po := db.po(swarm.NewAddress(addr))
		if po > swarm.MaxPO {
			po = swarm.MaxPO
		}
		return int(po)
	}

	chunksIndex, err := db.shed.NewIndex("BatchID|PO|Hash->nil", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			key = make([]byte, 65)
			copy(key[:32], fields.BatchID)
			key[32] = uint8(pof(fields.Address))
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

	countsIndex, err := db.shed.NewIndex("BatchID->reserveRadius|counts", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.BatchID, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BatchID = key[:32]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			return append([]byte{fields.Radius}, fields.Counts.Counts...), nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Radius = value[0]
			e.Counts = &shed.Counts{Counts: value[1:]}
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	return &postageBatches{
		chunks:     chunksIndex,
		counts:     countsIndex,
		po:         pof,
		batchStore: batchStore,
		db:         db,
	}, nil
}

func (p *postageBatches) decInBatch(batch *leveldb.Batch, e shed.Item) (bool, error) {
	item, err := p.counts.Get(e)
	if err != nil {
		return false, err
	}
	for i := 0; i < p.po(item.Address); i++ {
		count := item.Counts.Dec(i)
		if count == 0 { // if 0 then all subsequent counts are 0 too
			if i == 0 { // if all counts 0 the entire batch entry can be deleted
				return true, nil
			}
			break
		}
	}
	return false, p.counts.PutInBatch(batch, item)
}

func (p *postageBatches) incInBatch(batch *leveldb.Batch, e shed.Item) error {
	item, err := p.counts.Get(e)
	if err != nil {
		// initialise counts
		if !errors.Is(err, leveldb.ErrNotFound) {
			return err
		}
		e.Counts = &shed.Counts{Counts: make([]byte, swarm.MaxPO*4+4)}
		item = e
	}
	// retrieve batch - if not found or expired -> invalid
	b, err := p.batchStore.Get(item.BatchID)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return ErrBatchNotFound
		}
		return err
	}
	depth := int(b.Depth)
	po := p.po(item.Address)
	// increment counts
	for i := 0; i <= po; i++ {
		count := item.Counts.Inc(i)
		// counts track batch number of stamps in the batch for neighbourhoods of all depths
		// if neighbourhood_depth > batch_depth then the batch itself is invalid
		if order := depth - i; order >= 0 {
			if count > 1<<order {
				return ErrBatchOverissued
			}
		}
	}
	return p.counts.PutInBatch(batch, item)
}

func (p *postageBatches) putInBatch(batch *leveldb.Batch, item shed.Item) error {
	err := p.chunks.PutInBatch(batch, item)
	if err != nil {
		return err
	}
	return p.incInBatch(batch, item)
}

func (p *postageBatches) deleteInBatch(batch *leveldb.Batch, item shed.Item) error {
	err := p.chunks.DeleteInBatch(batch, item)
	if err != nil {
		return err
	}
	empty, err := p.decInBatch(batch, item)
	if err != nil {
		return err
	}
	if empty {
		return p.counts.DeleteInBatch(batch, item)
	}
	return nil
}

// unreserveBatch atomically unpins  chunks of a batch in proximity order upto and including po
// and marks the batch pinned within radius po
// if batch is marked as pinned within radius r>po, then do nothing
// unpinning will result in all chunks  with pincounter 0 to be put in the gc index
// so if a chunk was only pinned by the reserve, unreserving it  will make it gc-able
func (p *postageBatches) unreserveBatch(id []byte, po uint8) error {
	p.db.batchMu.Lock()
	defer p.db.batchMu.Unlock()

	batch := new(leveldb.Batch)
	var gcSizeChange int64 // number to add or subtract from gcSize
	item, err := p.counts.Get(shed.Item{BatchID: id})
	if err != nil {
		return err
	}
	unpin := func(item shed.Item) (stop bool, err error) {
		c, err := p.db.setUnpin(batch, swarm.NewAddress(item.Address))
		gcSizeChange += c
		return false, err
	}
	// iterate over chunk in bins
	for bin := item.Radius; bin < po; bin++ {
		err = p.chunks.Iterate(unpin, &shed.IterateOptions{Prefix: append(id, bin)})
		if err != nil {
			return err
		}
	}
	//  adjust gcSize
	err = p.db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return err
	}
	// save batch with new reserve radius
	item.Radius = po
	err = p.counts.PutInBatch(batch, item)
	if err != nil {
		return err
	}
	return p.db.shed.WriteBatch(batch)
}
