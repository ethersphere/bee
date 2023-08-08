// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/migration"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/swarm"
)

// AllSteps lists all migration steps for localstore IndexStore.
func AllSteps() migration.Steps {
	return map[uint64]migration.StepFn{
		1: step_01,
		2: step_02,
	}
}

func migrateBinIDs(st storage.Store) error {

	/*
		STEP 1, remove all of the BinItem entired
		STEP 2, iterate ChunkBinItem, delete old ChunkBinItem and BatchRadiusItem
		STEP 3, get new binID using IncBinID, create new ChunkBinItem and BatchRadiusItem
	*/

	rs := reserve.Reserve{}

	// STEP 1
	for i := uint8(0); i < swarm.MaxBins; i++ {
		err := st.Delete(&reserve.BinItem{Bin: i})
		if err != nil {
			return err
		}
	}

	return st.Iterate(
		storage.Query{
			Factory:       func() storage.Item { return &reserve.ChunkBinItem{} },
			PrefixAtStart: true,
		},
		func(res storage.Result) (bool, error) {

			item := res.Entry.(*reserve.ChunkBinItem)

			//STEP 2
			st.Delete(item)
			st.Delete(&reserve.BatchRadiusItem{BatchID: item.BatchID, Bin: item.Bin, Address: item.Address})

			newBinID, _ := rs.IncBinID(st, item.Bin)

			//STEP 3

			item.BinID = newBinID
			st.Put(item)
			st.Put(&reserve.BatchRadiusItem{BatchID: item.BatchID, Bin: item.Bin, Address: item.Address, BinID: newBinID})

			return false, nil
		},
	)
}
