// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	_ "embed"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/statestore/storeadapter"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/cache"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ldb "github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

//go:embed batches/batchstore_embed.json
var embeddedBatchstoreImportData []byte

const (
	stateStoreAdapterPrefix = "ss/"

	batchstoreBatchKeyPrefix = "batchstore_batch_"
	batchstoreValueKeyPrefix = "batchstore_value_"
)

// batchKeyForLevelDB constructs the full raw key used in LevelDB for a batch entry.
func batchKeyForLevelDB(batchID []byte) []byte {
	return []byte(stateStoreAdapterPrefix + batchstoreBatchKeyPrefix + string(batchID))
}

// batchDBPrefix returns the raw LevelDB key prefix used for batch entries.
func batchDBPrefix() []byte {
	return []byte(stateStoreAdapterPrefix + batchstoreBatchKeyPrefix)
}

// valueKeyForLevelDB constructs the full raw key used in LevelDB for a batch value index entry.
func valueKeyForLevelDB(val *big.Int, batchID []byte) []byte {
	valueBytes := make([]byte, 32)
	val.FillBytes(valueBytes)
	innerKey := batchstoreValueKeyPrefix + string(valueBytes) + string(batchID)
	return []byte(stateStoreAdapterPrefix + innerKey)
}

// InitStateStore will initialize the stateStore with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStateStore(logger log.Logger, dataDir string, cacheCapacity uint64) (storage.StateStorerManager, metrics.Collector, uint64, error) {
	var (
		statestorePath          string
		statestoreExistedBefore bool
		maxImportedBatchStart   uint64 = 0
		err                     error
	)

	if dataDir == "" {
		logger.Warning("using in-mem state store, no node state will be persisted")
		statestorePath = ""
		statestoreExistedBefore = false
	} else {
		statestorePath = filepath.Join(dataDir, "statestore")
		_, statErr := os.Stat(statestorePath)
		if statErr == nil {
			statestoreExistedBefore = true
		} else if os.IsNotExist(statErr) {
			statestoreExistedBefore = false
		} else {
			return nil, nil, 0, fmt.Errorf("stat %s: %w", statestorePath, statErr)
		}
	}

	// Initialize the underlying LevelDB store
	ldbStore, err := leveldbstore.New(statestorePath, nil)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("init leveldbstore: %w", err)
	}

	var successfulInit bool = false
	defer func() {
		if !successfulInit {
			if cerr := ldbStore.Close(); cerr != nil {
				logger.Error(cerr, "failed closing state store after init error")
			}
		}
	}()

	// --- Batch Import Logic ---
	if dataDir != "" { // Only perform checks and imports for persistent stores
		rawDB := ldbStore.DB()
		shouldImportBatches := false

		// Check if any batch data already exists in the DB using the helper function
		batchesExist, checkErr := hasExistingBatches(rawDB)
		if checkErr != nil {
			err = fmt.Errorf("failed to check for existing batches: %w", checkErr) // Assign to outer err
			return nil, nil, 0, err                                                // Return error, defer will close ldbStore
		}

		if !batchesExist {
			// No batches found in the database. Trigger import.
			shouldImportBatches = true
			if statestoreExistedBefore {
				logger.Info("statestore exists but contains no batches, attempting import from embedded batch data...")
			} else {
				logger.Info("new statestore detected, attempting import from embedded batch data...")
			}
		} else {
			logger.Info("statestore already contains batch data, skipping import.")
		}

		// Perform import if needed
		if shouldImportBatches {
			if len(embeddedBatchstoreImportData) > 0 {
				maxImportedBatchStart, err = importBatchesFromData(logger, rawDB, embeddedBatchstoreImportData, "embedded")
				if err != nil {
					err = fmt.Errorf("embedded batchstore import failed: %w", err) // Assign to outer err
					return nil, nil, 0, err                                        // Return error, defer will close ldbStore
				}
			} else {
				logger.Info("statestore requires import, but no embedded batch data found. Skipping import.")
			}
		}
	}
	// --- End Batch Import Logic ---

	// Wrap in cache
	caching, err := cache.Wrap(ldbStore, int(cacheCapacity))
	if err != nil {
		err = fmt.Errorf("wrap leveldbstore in cache: %w", err) // Assign to outer err
		return nil, nil, 0, err                                 // Return error, defer will close ldbStore
	}

	// Create adapter
	stateStore, err := storeadapter.NewStateStorerAdapter(caching)
	if err != nil {
		err = fmt.Errorf("create store adapter: %w", err) // Assign to outer err
		return nil, nil, 0, err                           // Return error, defer will close ldbStore
	}

	successfulInit = true // Mark init as successful, preventing deferred close
	return stateStore, caching, maxImportedBatchStart, nil
}

// hasExistingBatches checks if any key with the batch prefix exists in the LevelDB database.
func hasExistingBatches(db *ldb.DB) (bool, error) {
	prefix := batchDBPrefix()
	// Create an iterator for the given prefix. Don't fill the cache for this check.
	iter := db.NewIterator(util.BytesPrefix(prefix), &opt.ReadOptions{DontFillCache: true})
	defer iter.Release() // Ensure iterator is always released

	// Try to move to the first key matching the prefix.
	// iter.Next() returns true if a key was found, false otherwise.
	found := iter.Next()

	// Check for any errors during iterator creation or the Next() call.
	if err := iter.Error(); err != nil {
		return false, fmt.Errorf("iterator error during batch check: %w", err)
	}

	// Return whether a key was found and no error occurred.
	return found, nil
}

func importBatchesFromData(logger log.Logger, rawDB *ldb.DB, data []byte, source string) (uint64, error) {
	logger.Info("starting batch import process...", "source", source)
	startTime := time.Now()
	var (
		importedCount uint64
		skippedCount  uint64
		maxStartBlock uint64 = 0 // Initialize max start block
	)

	dataReader := bytes.NewReader(data)
	scanner := bufio.NewScanner(dataReader)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue // Skip empty lines
		}

		var batch postage.Batch
		if err := json.Unmarshal(line, &batch); err != nil {
			logger.Warning("failed to unmarshal line in batch import data, skipping line", "source", source, "error", err, "line_snippet", string(line[:min(len(line), 100)]))
			skippedCount++
			continue
		}

		// Validate batch ID length (basic sanity check)
		if len(batch.ID) != 32 {
			logger.Warning("imported batch has invalid ID length, skipping", "source", source, "line_snippet", string(line[:min(len(line), 100)]))
			skippedCount++
			continue
		}

		if batch.Start > maxStartBlock {
			maxStartBlock = batch.Start
		}

		// Marshal batch back to binary for storage
		marshaledBatch, marshalErr := batch.MarshalBinary()
		if marshalErr != nil {
			logger.Warning("failed to marshal imported batch to binary, skipping batch", "source", source, "batch_id", hex.EncodeToString(batch.ID), "error", marshalErr)
			skippedCount++
			continue
		}

		// Construct the keys including the necessary prefixes
		levelDbKeyBatch := batchKeyForLevelDB(batch.ID)
		levelDbKeyValue := valueKeyForLevelDB(batch.Value, batch.ID)

		// --- Use LevelDB Batch for efficiency ---
		ldbBatch := new(ldb.Batch)
		ldbBatch.Put(levelDbKeyBatch, marshaledBatch)
		ldbBatch.Put(levelDbKeyValue, []byte{}) // Value index key has empty value

		if errWrite := rawDB.Write(ldbBatch, nil); errWrite != nil {
			return maxStartBlock, fmt.Errorf("failed to write batch %s to leveldb (source: %s): %w", hex.EncodeToString(batch.ID), source, errWrite)
		}

		importedCount++
		if importedCount > 0 && importedCount%5000 == 0 {
			logger.Debug("batch import progress", "source", source, "imported", importedCount, "skipped", skippedCount, "elapsed", time.Since(startTime).Round(time.Millisecond))
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		logger.Warning("error scanning batch import data", "source", source, "error", scanErr)
	}

	logger.Info("batch import process finished",
		"source", source,
		"imported_count", importedCount,
		"skipped_count", skippedCount,
		"total_time", time.Since(startTime).Round(time.Millisecond),
	)
	return maxStartBlock, scanner.Err()
}

// InitStamperStore will create new stamper store with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStamperStore(logger log.Logger, dataDir string, stateStore storage.StateStorer) (storage.Store, error) {
	if dataDir == "" {
		logger.Warning("using in-mem stamper store, no node state will be persisted")
	} else {
		dataDir = filepath.Join(dataDir, "stamperstore")
	}
	stamperStore, err := leveldbstore.New(dataDir, nil)
	if err != nil {
		return nil, err
	}

	return stamperStore, nil
}

const (
	overlayNonce     = "overlayV2_nonce"
	noncedOverlayKey = "nonce-overlay"
)

// checkOverlay checks the overlay is the same as stored in the statestore
func checkOverlay(storer storage.StateStorer, overlay swarm.Address) error {

	var storedOverlay swarm.Address
	err := storer.Get(noncedOverlayKey, &storedOverlay)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		return storer.Put(noncedOverlayKey, overlay)
	}

	if !storedOverlay.Equal(overlay) {
		return fmt.Errorf("overlay address changed. was %s before but now is %s", storedOverlay, overlay)
	}

	return nil
}

func overlayNonceExists(s storage.StateStorer) ([]byte, bool, error) {
	nonce := make([]byte, 32)
	if err := s.Get(overlayNonce, &nonce); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nonce, false, nil
		}
		return nil, false, err
	}
	return nonce, true, nil
}

func setOverlay(s storage.StateStorer, overlay swarm.Address, nonce []byte) error {
	return errors.Join(
		s.Put(overlayNonce, nonce),
		s.Put(noncedOverlayKey, overlay),
	)
}
