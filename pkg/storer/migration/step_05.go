// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"fmt"
	"os"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
)

// step_05 is a migration step that removes all upload items from the store.
func step_05(st storage.BatchedStore) error {
	logger := log.NewLogger("migration-step-05", log.WithSink(os.Stdout))
	logger.Info("start removing upload items")

	itemC := make(chan storage.Item)
	errC := make(chan error)
	go func() {
		for item := range itemC {
			err := st.Delete(item)
			if err != nil {
				errC <- fmt.Errorf("delete upload item: %w", err)
				return
			}
		}
		close(errC)
	}()

	go func() {
		defer close(itemC)
		err := upload.IterateAll(st, func(u storage.Item) (bool, error) {
			itemC <- u
			return false, nil
		})
		if err != nil {
			errC <- fmt.Errorf("iterate upload items: %w", err)
			return
		}
	}()

	err := <-errC
	if err != nil {
		return err
	}

	logger.Info("finished removing upload items")
	return nil
}
