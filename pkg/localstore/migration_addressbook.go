// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// DBSchemaResidue is the bee schema identifier for residual migration.
const DBSchemaAddressBook = "address-book"

// migrateResidue sanitizes the pullIndex by removing any entries that are present
// in gcIndex from the pullIndex.
func migrateAddressBook(db *DB) error {
	db.logger.Info("starting address book migration")
	start := time.Now()
	var err error

	updateBatch := new(leveldb.Batch)
	updatedCount := 0

	err = addressbook.Iterate(func(item shed.Item) (bool, error) {

		return false, nil
	}, nil)
	if err != nil {
		return err
	}

	err = db.shed.WriteBatch(updateBatch)
	if err != nil {
		return fmt.Errorf("failed to update entries: %w", err)
	}

	db.logger.Info("residual migration done", "elapsed", time.Since(start), "cleaned_pull_indexes", updatedCount)
	return nil
}
