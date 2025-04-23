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
)

// go:embed batches/batchstore_embed.json
var embeddedBatchstoreImportData []byte

const (
	// Prefix used by storeadapter for StateStorer keys
	stateStoreAdapterPrefix = "ss/"

	// Prefixes used by batchstore WITHIN the state store
	batchstoreBatchKeyPrefix = "batchstore_batch_"
	batchstoreValueKeyPrefix = "batchstore_value_"

	// Filename for optional EXTERNAL batchstore import override
	batchstoreExternalImportFilename = "batchstore_import.json"
)

// batchKeyForLevelDB constructs the full raw key used in LevelDB for a batch entry.
func batchKeyForLevelDB(batchID []byte) []byte {
	return []byte(stateStoreAdapterPrefix + batchstoreBatchKeyPrefix + string(batchID))
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
func InitStateStore(logger log.Logger, dataDir string, cacheCapacity uint64) (storage.StateStorerManager, metrics.Collector, error) {
	var statestorePath string
	var statestoreExistedBefore bool
	var err error

	if dataDir == "" {
		logger.Warning("using in-mem state store, no node state will be persisted")
		statestorePath = ""
		statestoreExistedBefore = false
	} else {
		statestorePath = filepath.Join(dataDir, "statestore")
		_, err = os.Stat(statestorePath)
		if err == nil {
			statestoreExistedBefore = true
		} else if os.IsNotExist(err) {
			statestoreExistedBefore = false
		} else {
			return nil, nil, fmt.Errorf("stat %s: %w", statestorePath, err)
		}
	}

	// Initialize the underlying LevelDB store
	ldb, err := leveldbstore.New(statestorePath, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("init leveldbstore: %w", err)
	}

	// Attempt import ONLY if statestore is newly created AND not in-memory
	if !statestoreExistedBefore && dataDir != "" {
		rawDB := ldb.DB() // Get raw DB for direct import writes

		// --- Check ONLY embedded data ---
		if len(embeddedBatchstoreImportData) > 0 {
			logger.Info("new statestore detected, attempting import from embedded batch data...")
			err = importBatchesFromData(logger, rawDB, embeddedBatchstoreImportData, "embedded") // Pass embedded data
			if err != nil {
				_ = ldb.Close() // Close DB if import fails critically
				return nil, nil, fmt.Errorf("embedded batchstore import failed: %w", err)
			}
		} else {
			logger.Info("new statestore detected, but no embedded batch data found. Skipping import.")
		}
	}

	caching, err := cache.Wrap(ldb, int(cacheCapacity))
	if err != nil {
		_ = ldb.Close()
		return nil, nil, fmt.Errorf("wrap leveldbstore in cache: %w", err)
	}

	stateStore, err := storeadapter.NewStateStorerAdapter(caching)
	if err != nil {
		_ = ldb.Close()
		return nil, nil, fmt.Errorf("create store adapter: %w", err)
	}

	return stateStore, caching, nil
}

func importBatchesFromData(logger log.Logger, rawDB *ldb.DB, data []byte, source string) error {
	logger.Info("starting batch import process...", "source", source)
	startTime := time.Now()
	var importedCount uint64
	var skippedCount uint64

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
			// Return error to stop the entire import process if DB write fails
			return fmt.Errorf("failed to write batch %s to leveldb (source: %s): %w", hex.EncodeToString(batch.ID), source, errWrite)
		}
		// --- End LevelDB Batch ---

		importedCount++
		if importedCount > 0 && importedCount%5000 == 0 {
			logger.Debug("batch import progress", "source", source, "imported", importedCount, "skipped", skippedCount, "elapsed", time.Since(startTime).Round(time.Millisecond))
		}
	}

	// Check for scanner errors (e.g., incomplete data)
	if scanErr := scanner.Err(); scanErr != nil {
		// Log as a warning, as some data might have been imported successfully
		logger.Warning("error scanning batch import data", "source", source, "error", scanErr)
	}

	logger.Info("batch import process finished",
		"source", source,
		"imported_count", importedCount,
		"skipped_count", skippedCount,
		"total_time", time.Since(startTime).Round(time.Millisecond),
	)
	return nil // Import successful or finished with non-critical errors
}

// Helper for logging snippets
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
