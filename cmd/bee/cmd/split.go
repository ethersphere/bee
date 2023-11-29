// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

// putter is a putter that stores all the split chunk addresses of a file
type putter struct {
	rootHash       string
	chunkAddresses []string
}

func (s *putter) Put(ctx context.Context, chunk swarm.Chunk) error {
	s.chunkAddresses = append(s.chunkAddresses, chunk.Address().String())
	return nil
}

var _ storage.Putter = (*putter)(nil)

func (c *command) initSplitCmd() error {
	optionNameInputFile := "input-file"
	optionNameOutputFile := "output-file"
	cmd := &cobra.Command{
		Use:   "split",
		Short: "Split a file into a list chunks. The 1st line is the root hash",
		RunE: func(cmd *cobra.Command, args []string) error {
			inputFileName, err := cmd.Flags().GetString(optionNameInputFile)
			if err != nil {
				return fmt.Errorf("get input file name: %w", err)
			}
			outputFileName, err := cmd.Flags().GetString(optionNameOutputFile)
			if err != nil {
				return fmt.Errorf("get output file name: %w", err)
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

			reader, err := os.Open(inputFileName)
			if err != nil {
				return fmt.Errorf("open input file: %w", err)
			}
			defer reader.Close()

			logger.Info("splitting", "file", inputFileName)
			store := new(putter)
			s := splitter.NewSimpleSplitter(store)
			stat, err := reader.Stat()
			if err != nil {
				return fmt.Errorf("stat file: %w", err)
			}
			rootHash, err := file.SplitWriteAll(context.Background(), s, reader, stat.Size(), false)
			if err != nil {
				return fmt.Errorf("split write: %w", err)
			}
			store.rootHash = rootHash.String()

			logger.Info("writing output", "file", outputFileName)
			writer, err := os.OpenFile(outputFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("open output file: %w", err)
			}
			defer writer.Close()

			logger.Debug("write root", "hash", store.rootHash)
			_, err = writer.WriteString(fmt.Sprintf("%s\n", store.rootHash))
			if err != nil {
				return fmt.Errorf("write root hash: %w", err)
			}
			for _, chunkAddress := range store.chunkAddresses {
				logger.Debug("write chunk", "hash", chunkAddress)
				_, err = writer.WriteString(fmt.Sprintf("%s\n", chunkAddress))
				if err != nil {
					return fmt.Errorf("write chunk address: %w", err)
				}
			}
			logger.Info("done", "hashes", len(store.chunkAddresses))
			return nil
		},
	}

	cmd.Flags().String(optionNameVerbosity, "info", "verbosity level")
	cmd.Flags().String(optionNameInputFile, "", "input file")
	cmd.Flags().String(optionNameOutputFile, "", "output file")
	cmd.MarkFlagsRequiredTogether(optionNameInputFile, optionNameOutputFile)

	c.root.AddCommand(cmd)
	return nil
}
