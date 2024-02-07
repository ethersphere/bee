// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"os"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// step_04 is the fourth step of the migration. It forces a sharky recovery to
// be run on the localstore.
func step_04(
	sharkyBasePath string,
	sharkyNoOfShards int,
	st transaction.Storage,
) func() error {
	return func() error {
		// for in-mem store, skip this step
		if sharkyBasePath == "" {
			return nil
		}
		logger := log.NewLogger("migration-step-04", log.WithSink(os.Stdout))

		logger.Info("starting sharky recovery")
		sharkyRecover, err := sharky.NewRecovery(sharkyBasePath, sharkyNoOfShards, swarm.SocMaxChunkSize)
		if err != nil {
			return err
		}

		locationResultC := make(chan chunkstore.LocationResult)
		chunkstore.IterateLocations(context.Background(), st.IndexStore(), locationResultC)

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
		logger.Info("finished sharky recovery")

		return sharkyRecover.Close()
	}
}
