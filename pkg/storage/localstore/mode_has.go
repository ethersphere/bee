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

	"github.com/ethersphere/bee/pkg/swarm"
)

// Has returns true if the chunk is stored in database.
func (db *DB) Has(ctx context.Context, addr swarm.Address) (bool, error) {

	db.metrics.ModeHas.Inc()
	defer totalTimeMetric(db.metrics.TotalTimeHas, time.Now())

	has, err := db.retrievalDataIndex.Has(addressToItem(addr))
	if err != nil {
		db.metrics.ModeHasFailure.Inc()
	}
	return has, err
}

// HasMulti returns a slice of booleans which represent if the provided chunks
// are stored in database.
func (db *DB) HasMulti(ctx context.Context, addrs ...swarm.Address) ([]bool, error) {

	db.metrics.ModeHasMulti.Inc()
	defer totalTimeMetric(db.metrics.TotalTimeHasMulti, time.Now())

	have, err := db.retrievalDataIndex.HasMulti(addressesToItems(addrs...)...)
	if err != nil {
		db.metrics.ModeHasMultiFailure.Inc()
	}
	return have, err
}
