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
	ErrBatchNotFound   = errors.New("postage batch not found or expired")
)

// BatchStore interface to
type BatchStore interface {
	Get(id []byte) (*postage.Batch, error)
}

type postageBatches struct {
	// postage batch to chunks index
	chunks     shed.Index
	counts     shed.Index
	po         func(itemAddr []byte) (bin int)
	batchStore BatchStore
}

func (db *DB) newPostageBatches(batchStore BatchStore) (*postageBatches, error) {

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

	chunksIndex, err := db.shed.NewIndex("BatchID|Hash->nil", shed.IndexFuncs{
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

	countsIndex, err := db.shed.NewIndex("BatchID->counts", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.BatchID, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.BatchID = key[:32]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			return fields.Counts.Counts, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Counts = &shed.Counts{Counts: value}
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
