// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/ioutil"
	"github.com/spf13/cobra"
)

const (
	optionNameValidation     = "validate"
	optionNameValidationPin  = "validate-pin"
	optionNameCollectionPin  = "pin"
	optionNameOutputLocation = "output"
)

func (c *command) initDBCmd() {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Perform basic DB related operations",
	}

	dbExportCmd(cmd)
	dbImportCmd(cmd)
	dbNukeCmd(cmd)
	dbInfoCmd(cmd)
	dbCompactCmd(cmd)
	dbValidateCmd(cmd)
	dbValidatePinsCmd(cmd)
	dbRepairReserve(cmd)

	c.root.AddCommand(cmd)
}

func dbInfoCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "info",
		Short: "Prints the different indexes present in the Database",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			start := time.Now()
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Info("getting db indices with data-dir", "path", dataDir)

			db, err := storer.New(cmd.Context(), dataDir, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
				CacheCapacity:   1_000_000,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}
			defer db.Close()

			logger.Info("getting db info", "path", dataDir)

			info, err := db.DebugInfo(cmd.Context())
			if err != nil {
				return fmt.Errorf("fetching db info: %w", err)
			}

			logger.Info("reserve", "size_within_radius", info.Reserve.SizeWithinRadius, "total_size", info.Reserve.TotalSize, "capacity", info.Reserve.Capacity)
			logger.Info("cache", "size", info.Cache.Size, "capacity", info.Cache.Capacity)
			logger.Info("chunk", "total", info.ChunkStore.TotalChunks, "shared", info.ChunkStore.SharedSlots)
			logger.Info("pinning", "chunks", info.Pinning.TotalChunks, "collections", info.Pinning.TotalCollections)
			logger.Info("upload", "uploaded", info.Upload.TotalUploaded, "synced", info.Upload.TotalSynced)
			logger.Info("done", "elapsed", time.Since(start))

			return nil
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	cmd.AddCommand(c)
}

func dbCompactCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "compact",
		Short: "Compacts the localstore sharky store.",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			d, err := cmd.Flags().GetDuration(optionNameSleepAfter)
			if err != nil {
				logger.Error(err, "getting sleep value failed")
			}
			defer func() {
				if d > 0 {
					logger.Info("command has finished, sleeping...", "duration", d.String())
					time.Sleep(d)
				}
			}()

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			validation, err := cmd.Flags().GetBool(optionNameValidation)
			if err != nil {
				return fmt.Errorf("get validation: %w", err)
			}

			logger.Warning("Compaction is a destructive process. If the process is stopped for any reason, the localstore may become corrupted.")
			logger.Warning("It is highly advised to perform the compaction on a copy of the localstore.")
			logger.Warning("After compaction finishes, the data directory may be replaced with the compacted version.")
			logger.Warning("you have another 10 seconds to change your mind and kill this process with CTRL-C...")
			time.Sleep(10 * time.Second)
			logger.Warning("proceeding with database compaction...")

			localstorePath := path.Join(dataDir, ioutil.DataPathLocalstore)

			err = storer.Compact(context.Background(), localstorePath, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
			}, validation)
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}

			return nil
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	c.Flags().Bool(optionNameValidation, false, "run chunk validation checks before and after the compaction")
	c.Flags().Duration(optionNameSleepAfter, time.Duration(0), "time to sleep after the operation finished")
	cmd.AddCommand(c)
}

func dbValidatePinsCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "validate-pin",
		Short: "Validates pin collection chunks with sharky store.",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			providedPin, err := cmd.Flags().GetString(optionNameCollectionPin)
			if err != nil {
				return fmt.Errorf("read pin option: %w", err)
			}

			outputLoc, err := cmd.Flags().GetString(optionNameOutputLocation)
			if err != nil {
				return fmt.Errorf("read location option: %w", err)
			}

			localstorePath := path.Join(dataDir, ioutil.DataPathLocalstore)

			err = storer.ValidatePinCollectionChunks(context.Background(), localstorePath, providedPin, outputLoc, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}

			return nil
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	c.Flags().String(optionNameCollectionPin, "", "only validate given pin")
	c.Flags().String(optionNameOutputLocation, "", "location and name of the output file")
	cmd.AddCommand(c)
}

func dbRepairReserve(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "repair-reserve",
		Short: "Repairs the reserve by resetting the binIDs and removes dangling entries.",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Warning("Repair will recreate the reserve entries based on the chunk availability in the chunkstore. The epoch time and bin IDs will be reset.")
			logger.Warning("The pullsync peer sync intervals are reset so on the next run, the node will perform historical syncing.")
			logger.Warning("This is a destructive process. If the process is stopped for any reason, the reserve may become corrupted.")
			logger.Warning("To prevent permanent loss of data, data should be backed up before running the cmd.")
			logger.Warning("You have another 10 seconds to change your mind and kill this process with CTRL-C...")
			time.Sleep(10 * time.Second)
			logger.Warning("proceeding with repair...")

			d, err := cmd.Flags().GetDuration(optionNameSleepAfter)
			if err != nil {
				logger.Error(err, "getting sleep value failed")
			}
			defer func() {
				if d > 0 {
					logger.Info("command has finished, sleeping...", "duration", d.String())
					time.Sleep(d)
				}
			}()

			db, err := storer.New(cmd.Context(), path.Join(dataDir, "localstore"), &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
				CacheCapacity:   1_000_000,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}
			defer db.Close()

			err = migration.ReserveRepairer(db.Storage(), storage.ChunkType, logger)()
			if err != nil {
				return fmt.Errorf("repair: %w", err)
			}

			stateStore, _, err := node.InitStateStore(logger, dataDir, 1000)
			if err != nil {
				return fmt.Errorf("new statestore: %w", err)
			}
			defer stateStore.Close()

			return stateStore.Iterate(puller.IntervalPrefix, func(key, val []byte) (stop bool, err error) {
				return false, stateStore.Delete(string(key))
			})
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	c.Flags().Duration(optionNameSleepAfter, time.Duration(0), "time to sleep after the operation finished")
	cmd.AddCommand(c)
}

func dbValidateCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "validate",
		Short: "Validates the localstore sharky store.",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Warning("Validation ensures that sharky returns a chunk that hashes to the expected reference.")
			logger.Warning("    Invalid chunks logged at Warning level.")
			logger.Warning("    Progress logged at Info level.")
			logger.Warning("    SOC chunks logged at Debug level.")

			localstorePath := path.Join(dataDir, ioutil.DataPathLocalstore)

			err = storer.ValidateRetrievalIndex(context.Background(), localstorePath, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}

			return nil
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	cmd.AddCommand(c)
}

func dbExportCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "export",
		Short: "Perform DB export to a file",
	}
	c.PersistentFlags().String(optionNameDataDir, "", "data directory")
	c.PersistentFlags().String(optionNameVerbosity, "info", "verbosity level")

	dbExportReserveCmd(c)
	dbExportPinningCmd(c)
	cmd.AddCommand(c)
}

type noopRadiusSetter struct{}

func (noopRadiusSetter) SetStorageRadius(uint8) {}

func dbExportReserveCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "reserve <filename>",
		Short: "Export reserve DB to a file. Use \"-\" as filename in order to write to STDOUT",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if (len(args)) != 1 {
				return cmd.Help()
			}
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Info("starting export process with data-dir", "path", dataDir)

			db, err := storer.New(cmd.Context(), dataDir, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
				CacheCapacity:   1_000_000,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}
			defer db.Close()

			var out io.Writer
			if args[0] == "-" {
				out = os.Stdout
			} else {
				f, err := os.Create(args[0])
				if err != nil {
					return fmt.Errorf("opening output file: %w", err)
				}
				defer f.Close()
				out = f
			}

			tw := tar.NewWriter(out)
			var counter int64
			err = db.ReserveIterateChunks(func(chunk swarm.Chunk) (stop bool, err error) {
				logger.Debug("exporting chunk", "address", chunk.Address().String())
				b, err := MarshalChunkToBinary(chunk)
				if err != nil {
					return true, fmt.Errorf("marshaling chunk: %w", err)
				}
				hdr := &tar.Header{
					Name: chunk.Address().String(),
					Size: int64(len(b)),
					Mode: 0600,
				}
				if err := tw.WriteHeader(hdr); err != nil {
					return true, fmt.Errorf("writing header: %w", err)
				}
				if _, err := tw.Write(b); err != nil {
					return true, fmt.Errorf("writing chunk: %w", err)
				}
				counter++
				return false, nil
			})
			if err != nil {
				return fmt.Errorf("exporting database: %w", err)
			}
			logger.Info("database exported successfully", "file", args[0], "total_records", counter)
			return nil
		},
	}
	cmd.AddCommand(c)
}

func dbExportPinningCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "pinning <filename>",
		Short: "Export pinning DB to a file. Use \"-\" as filename in order to write to STDOUT",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if (len(args)) != 1 {
				return cmd.Help()
			}
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Info("starting export process with data-dir", "path", dataDir)
			db, err := storer.New(cmd.Context(), dataDir, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
				CacheCapacity:   1_000_000,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}
			defer db.Close()

			var out io.Writer
			if args[0] == "-" {
				out = os.Stdout
			} else {
				f, err := os.Create(args[0])
				if err != nil {
					return fmt.Errorf("error opening output file: %w", err)
				}
				defer f.Close()
				out = f
			}

			tw := tar.NewWriter(out)
			pins, err := db.Pins()
			if err != nil {
				return fmt.Errorf("error getting pins: %w", err)
			}
			var nTotalChunks int64
			for _, root := range pins {
				var nChunks int64
				err = db.IteratePinCollection(root, func(addr swarm.Address) (stop bool, err error) {
					logger.Debug("exporting chunk", "root", root.String(), "address", addr.String())
					chunk, err := db.ChunkStore().Get(cmd.Context(), addr)
					if err != nil {
						return true, fmt.Errorf("error getting chunk: %w", err)
					}
					b, err := MarshalChunkToBinary(chunk)
					if err != nil {
						return true, fmt.Errorf("error marshaling chunk: %w", err)
					}
					err = tw.WriteHeader(&tar.Header{
						Name: root.String() + "/" + addr.String(),
						Size: int64(len(b)),
						Mode: 0600,
					})
					if err != nil {
						return true, fmt.Errorf("error writing header: %w", err)
					}
					_, err = tw.Write(b)
					if err != nil {
						return true, fmt.Errorf("error writing chunk: %w", err)
					}
					nChunks++
					return false, nil
				})
				if err != nil {
					return fmt.Errorf("error exporting database: %w", err)
				}
				nTotalChunks += nChunks
				logger.Info("exported collection successfully", "root", root.String(), "total_records", nChunks)
			}
			logger.Info("pinning database exported successfully", "file", args[0], "total_collections", len(pins), "total_records", nTotalChunks)
			return nil
		},
	}
	cmd.AddCommand(c)
}

func dbImportCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "import",
		Short: "Perform DB import from a file",
	}
	c.PersistentFlags().String(optionNameDataDir, "", "data directory")
	c.PersistentFlags().String(optionNameVerbosity, "info", "verbosity level")

	dbImportReserveCmd(c)
	dbImportPinningCmd(c)
	cmd.AddCommand(c)
}

func dbImportReserveCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "reserve <filename>",
		Short: "Import reserve DB from a file. Use \"-\" as filename in order to read from STDIN",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if (len(args)) != 1 {
				return cmd.Help()
			}
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}
			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			fmt.Printf("starting import process with data-dir at %s\n", dataDir)

			db, err := storer.New(cmd.Context(), dataDir, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
				CacheCapacity:   1_000_000,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}
			defer db.Close()

			var in io.Reader
			if args[0] == "-" {
				in = os.Stdin
			} else {
				f, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("opening input file: %w", err)
				}
				defer f.Close()
				in = f
			}

			tr := tar.NewReader(in)
			var counter int64
			for {
				hdr, err := tr.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return fmt.Errorf("reading tar header: %w", err)
				}
				b := make([]byte, hdr.Size)
				if _, err := io.ReadFull(tr, b); err != nil {
					return fmt.Errorf("reading chunk: %w", err)
				}

				chunk, err := UnmarshalChunkFromBinary(b, hdr.Name)
				if err != nil {
					return fmt.Errorf("unmarshaling chunk: %w", err)
				}
				logger.Debug("importing chunk", "address", chunk.Address().String())
				if err := db.ReservePutter().Put(cmd.Context(), chunk); err != nil {
					return fmt.Errorf("error importing chunk: %w", err)
				}
				counter++
			}
			logger.Info("database imported successfully", "file", args[0], "total_records", counter)
			return nil
		},
	}

	cmd.AddCommand(c)
}

func dbImportPinningCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "pinning <filename>",
		Short: "Import pinning DB from a file. Use \"-\" as filename in order to read from STDIN",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if (len(args)) != 1 {
				return cmd.Help()
			}
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}
			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			fmt.Printf("starting import process with data-dir at %s\n", dataDir)

			db, err := storer.New(cmd.Context(), dataDir, &storer.Options{
				Logger:          logger,
				RadiusSetter:    noopRadiusSetter{},
				Batchstore:      new(postage.NoOpBatchStore),
				ReserveCapacity: storer.DefaultReserveCapacity,
				CacheCapacity:   1_000_000,
			})
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}
			defer db.Close()

			var in io.Reader
			if args[0] == "-" {
				in = os.Stdin
			} else {
				f, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("error opening input file: %w", err)
				}
				defer f.Close()
				in = f
			}

			tr := tar.NewReader(in)
			var nChunks int64
			collections := make(map[string]storer.PutterSession)
			for {
				hdr, err := tr.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return fmt.Errorf("error reading tar header: %w", err)
				}
				addresses := strings.Split(hdr.Name, "/")
				if len(addresses) != 2 {
					return fmt.Errorf("invalid address format: %s", hdr.Name)
				}
				rootAddr, chunkAddr := addresses[0], addresses[1]
				logger.Debug("importing pinning", "root", rootAddr, "chunk", chunkAddr)

				collection, ok := collections[rootAddr]
				if !ok {
					collection, err = db.NewCollection(cmd.Context())
					if err != nil {
						return fmt.Errorf("error creating collection: %w", err)
					}
					collections[rootAddr] = collection
				}

				b := make([]byte, hdr.Size)
				if _, err = io.ReadFull(tr, b); err != nil {
					return fmt.Errorf("error reading chunk: %w", err)
				}
				chunk, err := UnmarshalChunkFromBinary(b, addresses[1])
				if err != nil {
					return fmt.Errorf("error unmarshaling chunk: %w", err)
				}
				err = collection.Put(cmd.Context(), chunk)
				if err != nil {
					return fmt.Errorf("error putting chunk: %w", err)
				}
				nChunks++
			}

			for addr, collection := range collections {
				rootAddr, _ := swarm.ParseHexAddress(addr)
				err = collection.Done(rootAddr)
				if err != nil {
					return fmt.Errorf("pinning collection: %w", err)
				}
			}

			logger.Info("pinning database imported successfully", "file", args[0], "total_collections", len(collections), "total_records", nChunks)
			return nil
		},
	}
	cmd.AddCommand(c)
}

func dbNukeCmd(cmd *cobra.Command) {
	const (
		optionNameForgetOverlay = "forget-overlay"
		optionNameForgetStamps  = "forget-stamps"

		localstore   = ioutil.DataPathLocalstore
		kademlia     = ioutil.DataPathKademlia
		statestore   = "statestore"
		stamperstore = "stamperstore"
	)

	c := &cobra.Command{
		Use:   "nuke",
		Short: "Nuke the DB so that bee resyncs all data next time it boots up.",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}
			d, err := cmd.Flags().GetDuration(optionNameSleepAfter)
			if err != nil {
				logger.Error(err, "getting sleep value failed")
			}
			defer func() {
				if d > 0 {
					logger.Info("command has finished, sleeping...", "duration", d.String())
					time.Sleep(d)
				}
			}()

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %w", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Warning("starting to nuke the DB with data-dir", "path", dataDir)
			logger.Warning("this process will erase all persisted chunks in your local storage")
			logger.Warning("it will NOT discriminate any pinned content, in case you were wondering")
			logger.Warning("you have another 10 seconds to change your mind and kill this process with CTRL-C...")
			time.Sleep(10 * time.Second)
			logger.Warning("proceeding with database nuke...")

			dirsToNuke := []string{localstore, kademlia}
			for _, dir := range dirsToNuke {
				err = removeContent(filepath.Join(dataDir, dir))
				if err != nil {
					return fmt.Errorf("delete %s: %w", dir, err)
				}
			}

			forgetOverlay, err := cmd.Flags().GetBool(optionNameForgetOverlay)
			if err != nil {
				return fmt.Errorf("get forget overlay: %w", err)
			}

			if forgetOverlay {
				err = removeContent(filepath.Join(dataDir, statestore))
				if err != nil {
					return fmt.Errorf("remove statestore: %w", err)
				}
				err = removeContent(filepath.Join(dataDir, stamperstore))
				if err != nil {
					return fmt.Errorf("remove stamperstore: %w", err)
				}
				return nil
			}

			logger.Info("nuking statestore...")

			forgetStamps, err := cmd.Flags().GetBool(optionNameForgetStamps)
			if err != nil {
				return fmt.Errorf("get forget stamps: %w", err)
			}

			stateStore, _, err := node.InitStateStore(logger, dataDir, 1000)
			if err != nil {
				return fmt.Errorf("new statestore: %w", err)
			}
			defer stateStore.Close()

			stateStoreCleaner, ok := stateStore.(storage.StateStorerCleaner)
			if ok {
				err = stateStoreCleaner.Nuke()
				if err != nil {
					return fmt.Errorf("statestore nuke: %w", err)
				}
			}

			if forgetStamps {
				err = removeContent(filepath.Join(dataDir, stamperstore))
				if err != nil {
					return fmt.Errorf("remove stamperstore: %w", err)
				}
			}

			return nil
		}}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "trace", "verbosity level")
	c.Flags().Duration(optionNameSleepAfter, time.Duration(0), "time to sleep after the operation finished")
	c.Flags().Bool(optionNameForgetOverlay, false, "forget the overlay and deploy a new chequebook on next boot-up")
	c.Flags().Bool(optionNameForgetStamps, false, "forget the existing stamps belonging to the node. even when forgotten, they will show up again after a chain resync")
	cmd.AddCommand(c)
}

func removeContent(path string) error {
	dir, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()

	subpaths, err := dir.Readdirnames(0)
	if err != nil {
		return err
	}

	for _, sub := range subpaths {
		err = os.RemoveAll(filepath.Join(path, sub))
		if err != nil {
			return err
		}
	}
	return nil
}

func MarshalChunkToBinary(c swarm.Chunk) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	dataLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(dataLenBytes, uint32(len(c.Data())))
	buf.Write(dataLenBytes)
	buf.Write(c.Data())

	if c.Stamp() == nil {
		return buf.Bytes(), nil
	}
	stampBytes, err := c.Stamp().MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshalling stamp: %w", err)
	}
	buf.Write(stampBytes)
	return buf.Bytes(), nil
}

func UnmarshalChunkFromBinary(data []byte, address string) (swarm.Chunk, error) {
	buf := bytes.NewBuffer(data)

	dataLenBytes := make([]byte, 4)
	_, err := buf.Read(dataLenBytes)
	dataLen := binary.BigEndian.Uint32(dataLenBytes)
	if err != nil {
		return nil, fmt.Errorf("reading chunk data size: %w", err)
	}

	b := make([]byte, dataLen)
	_, err = buf.Read(b)
	if err != nil {
		return nil, fmt.Errorf("reading chunk data: %w", err)
	}

	addr, err := swarm.ParseHexAddress(address)
	if err != nil {
		return nil, fmt.Errorf("parsing address: %w", err)
	}
	chunk := swarm.NewChunk(addr, b)
	// stamp not present
	if buf.Len() == 0 {
		return chunk, nil
	}

	stampBytes := make([]byte, postage.StampSize)
	_, err = buf.Read(stampBytes)
	if err != nil {
		return nil, fmt.Errorf("reading stamp: %w", err)
	}
	stamp := new(postage.Stamp)
	err = stamp.UnmarshalBinary(stampBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling stamp: %w", err)
	}

	if buf.Len() != 0 {
		return nil, fmt.Errorf("buffer should be empty")
	}

	chunk = chunk.WithStamp(stamp)
	return chunk, nil
}
