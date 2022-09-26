// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/spf13/cobra"
)

const (
	optionNameForgetOverlay = "forget-overlay"
	optionNameForgetStamps  = "forget-stamps"
)

func (c *command) initDBCmd() {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Perform basic DB related operations",
	}

	dbExportCmd(cmd)
	dbImportCmd(cmd)
	dbNukeCmd(cmd)
	dbIndicesCmd(cmd)

	c.root.AddCommand(cmd)
}

func dbIndicesCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "indices",
		Short: "Prints the DB indices",
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

			path := filepath.Join(dataDir, "localstore")

			storer, err := localstore.New(path, nil, nil, nil, logger)
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}

			indices, err := storer.DebugIndices()
			if err != nil {
				return fmt.Errorf("error fetching indices: %w", err)
			}

			for k, v := range indices {
				logger.Info("localstore", "index", k, "value", v)
			}

			logger.Info("done", "elapsed", time.Since(start))

			return nil
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	cmd.AddCommand(c)
}

func dbExportCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "export <filename>",
		Short: "Perform DB export to a file. Use \"-\" as filename in order to write to STDOUT",
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

			path := filepath.Join(dataDir, "localstore")

			storer, err := localstore.New(path, nil, nil, nil, logger)
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}

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
			c, err := storer.Export(out)
			if err != nil {
				return fmt.Errorf("error exporting database: %w", err)
			}

			logger.Info("database exported successfully", "total_records", c)

			return nil
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	cmd.AddCommand(c)
}

func dbImportCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "import <filename>",
		Short: "Perform DB import from a file. Use \"-\" as filename in order to feed from STDIN",
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

			path := filepath.Join(dataDir, "localstore")

			storer, err := localstore.New(path, nil, nil, nil, logger)
			if err != nil {
				return fmt.Errorf("localstore: %w", err)
			}

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
			c, err := storer.Import(cmd.Context(), in)
			if err != nil {
				return fmt.Errorf("error importing database: %w", err)
			}

			fmt.Printf("database imported %d records successfully\n", c)

			return nil
		},
	}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	cmd.AddCommand(c)
}

func dbNukeCmd(cmd *cobra.Command) {
	c := &cobra.Command{
		Use:   "nuke",
		Short: "Nuke the DB and the relevant statestore entries so that bee resyncs all data next time it boots up.",
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

			defer func() { time.Sleep(d) }()

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

			localstorePath := filepath.Join(dataDir, "localstore")
			err = removeContent(localstorePath)
			if err != nil {
				return fmt.Errorf("localstore delete: %w", err)
			}

			statestorePath := filepath.Join(dataDir, "statestore")

			forgetOverlay, err := cmd.Flags().GetBool(optionNameForgetOverlay)
			if err != nil {
				return fmt.Errorf("get forget overlay: %w", err)
			}

			forgetStamps, err := cmd.Flags().GetBool(optionNameForgetStamps)
			if err != nil {
				return fmt.Errorf("get forget stamps: %w", err)
			}

			if forgetOverlay {
				err = removeContent(statestorePath)
				if err != nil {
					return fmt.Errorf("statestore delete: %w", err)
				}
				// all done, return early
				return nil
			}

			stateStore, err := leveldb.NewStateStore(statestorePath, logger)
			if err != nil {
				return fmt.Errorf("new statestore: %w", err)
			}

			logger.Warning("proceeding with statestore nuke...")

			if err = stateStore.Nuke(forgetStamps); err != nil {
				return fmt.Errorf("statestore nuke: %w", err)
			}
			return nil
		}}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "trace", "verbosity level")
	c.Flags().Bool(optionNameForgetOverlay, false, "forget the overlay and deploy a new chequebook on next bootup")
	c.Flags().Bool(optionNameForgetStamps, false, "forget the existing stamps belonging to the node. even when forgotten, they will show up again after a chain resync")
	c.Flags().Duration(optionNameSleepAfter, time.Duration(0), "time to sleep after the operation finished")
	cmd.AddCommand(c)
}

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
