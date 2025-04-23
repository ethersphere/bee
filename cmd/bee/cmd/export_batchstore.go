package cmd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	dbExportBatchKeyPrefix   = "ss/batchstore_batch_"
	batchstoreExportFilename = "pkg/node/batches/batchstore_embed.json"
)

// runExportBatchstore exports batchstore batch data as NDJSON (Newline Delimited JSON).
func runExportBatchstore(logger log.Logger, dataDir string) (err error) {
	statestorePath := filepath.Join(dataDir, "statestore")
	logger.Info("using statestore path", "path", statestorePath)

	if _, statErr := os.Stat(statestorePath); os.IsNotExist(statErr) {
		return fmt.Errorf("statestore path %q does not exist", statestorePath)
	}

	var (
		db      *leveldbstore.Store
		outFile *os.File
	)
	defer func() {
		if outFile != nil {
			logger.Debug("closing output file")
			if closeErr := outFile.Close(); closeErr != nil {
				logger.Error(closeErr, "failed to close output file cleanly")
				if err == nil {
					err = fmt.Errorf("close output file: %w", closeErr)
				}
			}
		}
		if db != nil {
			logger.Debug("closing database")
			if closeErr := db.Close(); closeErr != nil {
				logger.Error(closeErr, "failed to close database cleanly")
				if err == nil {
					err = fmt.Errorf("close database: %w", closeErr)
				}
			}
		}
	}()

	dbOpts := &opt.Options{
		ReadOnly:       true,
		ErrorIfMissing: true,
	}
	db, err = leveldbstore.New(statestorePath, dbOpts)
	if err != nil {
		return fmt.Errorf("failed to open statestore leveldb store at %q: %w", statestorePath, err)
	}
	logger.Info("statestore database opened successfully")

	// Ensure the output directory exists
	outputDir := filepath.Dir(batchstoreExportFilename)
	if err = os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %q: %w", outputDir, err)
	}

	outFile, err = os.Create(batchstoreExportFilename)
	if err != nil {
		return fmt.Errorf("failed to create output file %q: %w", batchstoreExportFilename, err)
	}
	encoder := json.NewEncoder(outFile)

	logger.Info("exporting batchstore batch data to NDJSON file", "file", batchstoreExportFilename)

	var batchCount uint64
	startTime := time.Now()

	logger.Info("exporting batches...")
	batchPrefixBytes := []byte(dbExportBatchKeyPrefix)

	ldb := db.DB()
	iterOpts := &opt.ReadOptions{DontFillCache: true}
	iter := ldb.NewIterator(util.BytesPrefix(batchPrefixBytes), iterOpts)
	defer iter.Release()

	for iter.Next() {
		key := make([]byte, len(iter.Key()))
		copy(key, iter.Key())
		value := make([]byte, len(iter.Value()))
		copy(value, iter.Value())

		var batch postage.Batch
		unmarshalErr := batch.UnmarshalBinary(value)
		if unmarshalErr != nil {
			batchID := hex.EncodeToString(bytes.TrimPrefix(key, batchPrefixBytes))
			logger.Warning("failed to unmarshal batch, skipping", "key", string(key), "batch_id", batchID, "error", unmarshalErr)
			continue
		}

		encodeErr := encoder.Encode(batch)
		if encodeErr != nil {
			return fmt.Errorf("failed to encode batch %s to JSON line: %w", hex.EncodeToString(batch.ID), encodeErr)
		}

		batchCount++
		if batchCount > 0 && batchCount%5000 == 0 {
			logger.Debug("exported batches", "count", batchCount, "elapsed", time.Since(startTime).Round(time.Second))
		}
	}

	if iterErr := iter.Error(); iterErr != nil {
		logger.Error(iterErr, "leveldb iteration for batches failed")
		if err == nil {
			err = fmt.Errorf("leveldb batch iteration failed: %w", iterErr)
		}
	}
	logger.Info("finished exporting batches", "count", batchCount)

	totalTime := time.Since(startTime)
	logger.Info("batchstore batch NDJSON export completed",
		"batches_exported", batchCount,
		"output_file", batchstoreExportFilename,
		"total_time", totalTime.Round(time.Millisecond),
	)

	return err
}
