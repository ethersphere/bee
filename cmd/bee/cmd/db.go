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

func (c *command) initDBCmd() {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Perform basic DB related operations",
	}

	dbExportCmd(cmd)
	dbImportCmd(cmd)

	c.root.AddCommand(cmd)
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
				return fmt.Errorf("get verbosity: %v", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %v", err)
			}

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %v", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Infof("starting export process with data-dir at %s", dataDir)

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
					return fmt.Errorf("error opening output file: %s", err)
				}
				defer f.Close()
				out = f
			}
			c, err := storer.Export(out)
			if err != nil {
				return fmt.Errorf("error exporting database: %v", err)
			}

			logger.Infof("database exported %d records successfully", c)

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
				return fmt.Errorf("get verbosity: %v", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %v", err)
			}
			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %v", err)
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
					return fmt.Errorf("error opening input file: %s", err)
				}
				defer f.Close()
				in = f
			}
			c, err := storer.Import(cmd.Context(), in)
			if err != nil {
				return fmt.Errorf("error importing database: %v", err)
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
				return fmt.Errorf("get verbosity: %v", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %v", err)
			}
			d, err := cmd.Flags().GetDuration(optionNameSleepAfter)
			if err != nil {
				logger.Errorf("getting sleep value: %v", err)
			}

			defer time.Sleep(d)

			dataDir, err := cmd.Flags().GetString(optionNameDataDir)
			if err != nil {
				return fmt.Errorf("get data-dir: %v", err)
			}
			if dataDir == "" {
				return errors.New("no data-dir provided")
			}

			logger.Warningf("Starting to nuke the DB with data-dir at %s", dataDir)
			logger.Warningf("This process will erase all persisted chunks in your local storage!")
			logger.Warningf("It will NOT discriminate any pinned content, in case you were wondering.")
			logger.Warningf("You have another 10 seconds to change your mind and kill this process with CTRL-C...")
			time.Sleep(10 * time.Second)
			logger.Warningf("Proceeding with database nuke...")

			localstorePath := filepath.Join(dataDir, "localstore")
			err = os.RemoveAll(localstorePath)
			if err != nil {
				return fmt.Errorf("localstore delete: %w", err)
			}

			statestorePath := filepath.Join(dataDir, "statestore")
			stateStore, err := leveldb.NewStateStore(statestorePath, logger)
			if err != nil {
				return fmt.Errorf("new statestore: %w", err)
			}
			if err = stateStore.Nuke(); err != nil {
				return fmt.Errorf("statestore nuke: %w", err)
			}
			return nil
		}}
	c.Flags().String(optionNameDataDir, "", "data directory")
	c.Flags().String(optionNameVerbosity, "trace", "verbosity level")
	c.Flags().Duration(optionNameSleepAfter, time.Duration(0), "time to sleep after the operation finished")
	cmd.AddCommand(c)
}
