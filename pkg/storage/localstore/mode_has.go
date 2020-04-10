// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethersphere/swarm/chunk"
)

// Has returns true if the chunk is stored in database.
func (db *DB) Has(ctx context.Context, addr chunk.Address) (bool, error) {
	metricName := "localstore/Has"

	metrics.GetOrRegisterCounter(metricName, nil).Inc(1)
	defer totalTimeMetric(metricName, time.Now())

	has, err := db.retrievalDataIndex.Has(addressToItem(addr))
	if err != nil {
		metrics.GetOrRegisterCounter(metricName+"/error", nil).Inc(1)
	}
	return has, err
}

// HasMulti returns a slice of booleans which represent if the provided chunks
// are stored in database.
func (db *DB) HasMulti(ctx context.Context, addrs ...chunk.Address) ([]bool, error) {
	metricName := "localstore/HasMulti"

	metrics.GetOrRegisterCounter(metricName, nil).Inc(1)
	defer totalTimeMetric(metricName, time.Now())

	have, err := db.retrievalDataIndex.HasMulti(addressesToItems(addrs...)...)
	if err != nil {
		metrics.GetOrRegisterCounter(metricName+"/error", nil).Inc(1)
	}
	return have, err
}
