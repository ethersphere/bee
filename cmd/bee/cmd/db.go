// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	//localstore "github.com/ethersphere/bee/pkg/_localstore"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

//const (
//	optionNameForgetOverlay = "forget-overlay"
//	optionNameForgetStamps  = "forget-stamps"
//)

func (c *command) initDBCmd() {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Perform basic DB related operations",
	}

	dbExportCmd(cmd)
	dbImportCmd(cmd)
	//dbNukeCmd(cmd)
	//dbIndicesCmd(cmd)

	c.root.AddCommand(cmd)
}

//func dbIndicesCmd(cmd *cobra.Command) {
//	c := &cobra.Command{
//		Use:   "indices",
//		Short: "Prints the DB indices",
//		RunE: func(cmd *cobra.Command, args []string) (err error) {
//			start := time.Now()
//			v, err := cmd.Flags().GetString(optionNameVerbosity)
//			if err != nil {
//				return fmt.Errorf("get verbosity: %w", err)
//			}
//			v = strings.ToLower(v)
//			logger, err := newLogger(cmd, v)
//			if err != nil {
//				return fmt.Errorf("new logger: %w", err)
//			}
//
//			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
//			if err != nil {
//				return fmt.Errorf("get data-dir: %w", err)
//			}
//			if dataDir == "" {
//				return errors.New("no data-dir provided")
//			}
//
//			logger.Info("getting db indices with data-dir", "path", dataDir)
//
//			path := filepath.Join(dataDir, "localstore")
//
//			storer, err := localstore.New(path, nil, nil, nil, logger)
//			if err != nil {
//				return fmt.Errorf("localstore: %w", err)
//			}
//
//			indices, err := storer.DebugIndices()
//			if err != nil {
//				return fmt.Errorf("error fetching indices: %w", err)
//			}
//
//			for k, v := range indices {
//				logger.Info("localstore", "index", k, "value", v)
//			}
//
//			logger.Info("done", "elapsed", time.Since(start))
//
//			return nil
//		},
//	}
//	c.Flags().String(optionNameDataDir, "", "data directory")
//	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
//	cmd.AddCommand(c)
//}

func dbExportCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "export",
		Short: "Perform DB export to a file",
	}
	c.PersistentFlags().String(optionNameDataDir, "", "data directory")
	c.PersistentFlags().String(optionNameVerbosity, "info", "verbosity level")

	dbExportReserveCmd(c)
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
				ReserveCapacity: 4_194_304,
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
			var counter int64
			err = db.ReserveIterateChunks(func(chunk swarm.Chunk) (stop bool, err error) {
				logger.Info("exporting chunk", "address", chunk.Address().String())
				b, err := MarshalChunkToBinary(chunk)
				if err != nil {
					return true, fmt.Errorf("error marshaling chunk: %w", err)
				}
				hdr := &tar.Header{
					Name: chunk.Address().String(),
					Size: int64(len(b)),
					Mode: 0600,
				}
				if err := tw.WriteHeader(hdr); err != nil {
					return true, fmt.Errorf("error writing header: %w", err)
				}
				if _, err := tw.Write(b); err != nil {
					return true, fmt.Errorf("error writing chunk: %w", err)
				}
				counter++
				return false, nil
			})
			if err != nil {
				return fmt.Errorf("error exporting database: %w", err)
			}
			logger.Info("database exported successfully", "file", args[0], "total_records", counter)
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
	cmd.AddCommand(c)
}

func dbImportReserveCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "reserve <filename>",
		Short: "Perform DB import from a . Use \"-\" as filename in order to read from STDIN",
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
				ReserveCapacity: 4_194_304,
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
			var counter int64
			for {
				hdr, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return fmt.Errorf("error reading tar header: %w", err)
				}
				b := make([]byte, hdr.Size)
				if _, err := io.ReadFull(tr, b); err != nil {
					return fmt.Errorf("error reading chunk: %w", err)
				}

				chunk, err := UnmarshalChunkFromBinary(b, hdr.Name)
				logger.Info("importing chunk", "address", chunk.Address().String())
				if err := db.ReservePut(cmd.Context(), chunk); err != nil {
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

//func dbNukeCmd(cmd *cobra.Command) {
//	c := &cobra.Command{
//		Use:   "nuke",
//		Short: "Nuke the DB and the relevant statestore entries so that bee resyncs all data next time it boots up.",
//		RunE: func(cmd *cobra.Command, args []string) (err error) {
//			v, err := cmd.Flags().GetString(optionNameVerbosity)
//			if err != nil {
//				return fmt.Errorf("get verbosity: %w", err)
//			}
//			v = strings.ToLower(v)
//			logger, err := newLogger(cmd, v)
//			if err != nil {
//				return fmt.Errorf("new logger: %w", err)
//			}
//			d, err := cmd.Flags().GetDuration(optionNameSleepAfter)
//			if err != nil {
//				logger.Error(err, "getting sleep value failed")
//			}
//
//			defer func() { time.Sleep(d) }()
//
//			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
//			if err != nil {
//				return fmt.Errorf("get data-dir: %w", err)
//			}
//			if dataDir == "" {
//				return errors.New("no data-dir provided")
//			}
//
//			logger.Warning("starting to nuke the DB with data-dir", "path", dataDir)
//			logger.Warning("this process will erase all persisted chunks in your local storage")
//			logger.Warning("it will NOT discriminate any pinned content, in case you were wondering")
//			logger.Warning("you have another 10 seconds to change your mind and kill this process with CTRL-C...")
//			time.Sleep(10 * time.Second)
//			logger.Warning("proceeding with database nuke...")
//
//			localstorePath := filepath.Join(dataDir, "localstore")
//			err = removeContent(localstorePath)
//			if err != nil {
//				return fmt.Errorf("localstore delete: %w", err)
//			}
//
//			statestorePath := filepath.Join(dataDir, "statestore")
//
//			forgetOverlay, err := cmd.Flags().GetBool(optionNameForgetOverlay)
//			if err != nil {
//				return fmt.Errorf("get forget overlay: %w", err)
//			}
//
//			forgetStamps, err := cmd.Flags().GetBool(optionNameForgetStamps)
//			if err != nil {
//				return fmt.Errorf("get forget stamps: %w", err)
//			}
//
//			if forgetOverlay {
//				err = removeContent(statestorePath)
//				if err != nil {
//					return fmt.Errorf("statestore delete: %w", err)
//				}
//				// all done, return early
//				return nil
//			}
//
//			stateStore, err := leveldb.NewStateStore(statestorePath, logger)
//			if err != nil {
//				return fmt.Errorf("new statestore: %w", err)
//			}
//
//			logger.Warning("proceeding with statestore nuke...")
//
//			if err = stateStore.Nuke(forgetStamps); err != nil {
//				return fmt.Errorf("statestore nuke: %w", err)
//			}
//			return nil
//		}}
//	c.Flags().String(optionNameDataDir, "", "data directory")
//	c.Flags().String(optionNameVerbosity, "trace", "verbosity level")
//	c.Flags().Bool(optionNameForgetOverlay, false, "forget the overlay and deploy a new chequebook on next bootup")
//	c.Flags().Bool(optionNameForgetStamps, false, "forget the existing stamps belonging to the node. even when forgotten, they will show up again after a chain resync")
//	c.Flags().Duration(optionNameSleepAfter, time.Duration(0), "time to sleep after the operation finished")
//	cmd.AddCommand(c)
//}

func removeContent(path string) error {
	dir, err := os.Open(path)
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

	stampBytes, err := c.Stamp().MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling stamp: %v", err)
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
		return nil, fmt.Errorf("error reading chunk data size: %v", err)
	}

	b := make([]byte, dataLen)
	_, err = buf.Read(b)
	if err != nil {
		return nil, fmt.Errorf("error reading chunk data: %w", err)
	}

	addr, err := swarm.ParseHexAddress(address)
	if err != nil {
		return nil, fmt.Errorf("error parsing address: %w", err)
	}

	stampBytes := make([]byte, postage.StampSize)
	_, err = buf.Read(stampBytes)
	if err != nil {
		return nil, fmt.Errorf("error reading stamp: %w", err)
	}
	stamp := new(postage.Stamp)
	err = stamp.UnmarshalBinary(stampBytes)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling stamp: %w", err)
	}

	if buf.Len() != 0 {
		return nil, fmt.Errorf("buffer should be empty")
	}

	chunk := swarm.NewChunk(addr, b).WithStamp(stamp)
	return chunk, nil
}
