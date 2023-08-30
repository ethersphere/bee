// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"

	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

// step_04 is the fourth step of the migration. It forces a sharky recovery to
// be run on the localstore.
func step_04(
	sharkyBasePath string,
	sharkyNoOfShards int,
) func(st storage.BatchedStore) error {
	return func(st storage.BatchedStore) error {

		if sharkyBasePath == "" {
			return nil
		}

		sharkyRecover, err := sharky.NewRecovery(sharkyBasePath, sharkyNoOfShards, swarm.SocMaxChunkSize)
		if err != nil {
			return err
		}

		locationResultC := make(chan chunkstore.LocationResult)
		chunkstore.IterateLocations(context.Background(), st, locationResultC)

		for res := range locationResultC {
			if res.Err != nil {
				return res.Err
			}

			if err := sharkyRecover.Add(res.Location); err != nil {
				return err
			}
		}

		if err := sharkyRecover.Save(); err != nil {
			return err
		}

		return sharkyRecover.Close()
	}
}
