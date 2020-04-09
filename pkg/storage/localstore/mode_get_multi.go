// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethersphere/swarm/chunk"
	"github.com/ethersphere/swarm/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// GetMulti returns chunks from the database. If one of the chunks is not found
// chunk.ErrChunkNotFound will be returned. All required indexes will be updated
// required by the Getter Mode. GetMulti is required to implement chunk.Store
// interface.
func (db *DB) GetMulti(ctx context.Context, mode chunk.ModeGet, addrs ...chunk.Address) (chunks []chunk.Chunk, err error) {
	metricName := fmt.Sprintf("localstore/GetMulti/%s", mode)

	metrics.GetOrRegisterCounter(metricName, nil).Inc(1)
	defer totalTimeMetric(metricName, time.Now())

	defer func() {
		if err != nil {
			metrics.GetOrRegisterCounter(metricName+"/error", nil).Inc(1)
		}
	}()

	out, err := db.getMulti(mode, addrs...)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, chunk.ErrChunkNotFound
		}
		return nil, err
	}
	chunks = make([]chunk.Chunk, len(out))
	for i, ch := range out {
		chunks[i] = chunk.NewChunk(ch.Address, ch.Data).WithPinCounter(ch.PinCounter)
	}
	return chunks, nil
}

// getMulti returns Items from the retrieval index
// and updates other indexes.
func (db *DB) getMulti(mode chunk.ModeGet, addrs ...chunk.Address) (out []shed.Item, err error) {
	out = make([]shed.Item, len(addrs))
	for i, addr := range addrs {
		out[i].Address = addr
	}

	err = db.retrievalDataIndex.Fill(out)
	if err != nil {
		return nil, err
	}

	switch mode {
	// update the access timestamp and gc index
	case chunk.ModeGetRequest:
		db.updateGCItems(out...)

	case chunk.ModeGetPin:
		err := db.pinIndex.Fill(out)
		if err != nil {
			return nil, err
		}

	// no updates to indexes
	case chunk.ModeGetSync:
	case chunk.ModeGetLookup:
	default:
		return out, ErrInvalidMode
	}
	return out, nil
}
