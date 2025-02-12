// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
)

// step_05 is a migration step that removes all upload items from the store.
func step_05(st transaction.Storage, logger log.Logger) func() error {
	return func() error {

		logger := logger.WithName("migration-step-05").Register()

		logger.Info("start removing upload items")

		itemC := make(chan storage.Item)
		errC := make(chan error)
		go func() {
			for item := range itemC {
				err := st.Run(context.Background(), func(s transaction.Store) error {
					return s.IndexStore().Delete(item)
				})
				if err != nil {
					errC <- fmt.Errorf("delete upload item: %w", err)
					return
				}
			}
			close(errC)
		}()

		err := upload.IterateAll(st.IndexStore(), func(u storage.Item) (bool, error) {
			select {
			case itemC <- u:
			case err := <-errC:
				return true, err
			}
			return false, nil
		})
		close(itemC)
		if err != nil {
			return err
		}

		logger.Info("finished removing upload items")
		return <-errC
	}

}
