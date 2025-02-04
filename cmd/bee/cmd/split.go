// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/spf13/cobra"
)

// putter is a putter that stores all the split chunk addresses of a file
type putter struct {
	cb func(chunk swarm.Chunk) error
}

func (s *putter) Put(_ context.Context, chunk swarm.Chunk) error {
	return s.cb(chunk)
}
func newPutter(cb func(ch swarm.Chunk) error) *putter {
	return &putter{
		cb: cb,
	}
}

var _ storage.Putter = (*putter)(nil)

type pipelineFunc func(context.Context, io.Reader) (swarm.Address, error)

func requestPipelineFn(s storage.Putter, encrypt bool, rLevel redundancy.Level) pipelineFunc {
	return func(ctx context.Context, r io.Reader) (swarm.Address, error) {
		pipe := builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
		return builder.FeedPipeline(ctx, pipe, r)
	}
}

func (c *command) initSplitCmd() error {
	cmd := &cobra.Command{
		Use:   "split",
		Short: "Split a file into chunks",
	}

	splitRefs(cmd)
	splitChunks(cmd)
	c.root.AddCommand(cmd)
	return nil
}

func splitRefs(cmd *cobra.Command) {
	optionNameInputFile := "input-file"
	optionNameOutputFile := "output-file"
	optionNameRedundancyLevel := "r-level"

	c := &cobra.Command{
		Use:   "refs",
		Short: "Write only the chunk reference to the output file",
		RunE: func(cmd *cobra.Command, args []string) error {
			inputFileName, err := cmd.Flags().GetString(optionNameInputFile)
			if err != nil {
				return fmt.Errorf("get input file name: %w", err)
			}
			outputFileName, err := cmd.Flags().GetString(optionNameOutputFile)
			if err != nil {
				return fmt.Errorf("get output file name: %w", err)
			}
			rLevel, err := cmd.Flags().GetInt(optionNameRedundancyLevel)
			if err != nil {
				return fmt.Errorf("get redundancy level: %w", err)
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

			logger.Info("splitting", "file", inputFileName, "rLevel", rLevel)
			logger.Info("writing output", "file", outputFileName)

			var refs []string
			store := newPutter(func(ch swarm.Chunk) error {
				refs = append(refs, ch.Address().String())
				return nil
			})
			writer, err := os.OpenFile(outputFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("open output file: %w", err)
			}
			defer writer.Close()

			p := requestPipelineFn(store, false, redundancy.Level(rLevel))
			ctx := redundancy.SetLevelInContext(cmd.Context(), redundancy.Level(rLevel))
			rootRef, err := p(ctx, reader)
			if err != nil {
				return fmt.Errorf("pipeline: %w", err)
			}

			logger.Debug("write root", "hash", rootRef)
			_, err = writer.WriteString(fmt.Sprintf("%s\n", rootRef))
			if err != nil {
				return fmt.Errorf("write root hash: %w", err)
			}
			for _, ref := range refs {
				logger.Debug("write chunk", "hash", ref)
				_, err = writer.WriteString(fmt.Sprintf("%s\n", ref))
				if err != nil {
					return fmt.Errorf("write chunk address: %w", err)
				}
			}
			logger.Info("done", "root", rootRef.String(), "chunks", len(refs))
			return nil
		},
	}

	c.Flags().String(optionNameInputFile, "", "input file")
	c.Flags().String(optionNameOutputFile, "", "output file")
	c.Flags().Int(optionNameRedundancyLevel, 0, "redundancy level")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")

	c.MarkFlagsRequiredTogether(optionNameInputFile, optionNameOutputFile)

	cmd.AddCommand(c)
}

func splitChunks(cmd *cobra.Command) {
	optionNameInputFile := "input-file"
	optionNameOutputDir := "output-dir"
	optionNameRedundancyLevel := "r-level"

	c := &cobra.Command{
		Use:   "chunks",
		Short: "Write the chunks to the output directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			inputFileName, err := cmd.Flags().GetString(optionNameInputFile)
			if err != nil {
				return fmt.Errorf("get input file name: %w", err)
			}
			outputDir, err := cmd.Flags().GetString(optionNameOutputDir)
			if err != nil {
				return fmt.Errorf("get output file name: %w", err)
			}
			info, err := os.Stat(outputDir)
			if err != nil {
				return fmt.Errorf("stat output dir: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("output dir %s is not a directory", outputDir)
			}
			rLevel, err := cmd.Flags().GetInt(optionNameRedundancyLevel)
			if err != nil {
				return fmt.Errorf("get redundancy level: %w", err)
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

			logger.Info("splitting", "file", inputFileName, "rLevel", rLevel)
			logger.Info("writing output", "dir", outputDir)

			var chunksCount atomic.Int64
			store := newPutter(func(chunk swarm.Chunk) error {
				filePath := filepath.Join(outputDir, chunk.Address().String())
				err := os.WriteFile(filePath, chunk.Data(), 0644)
				if err != nil {
					return err
				}
				chunksCount.Add(1)
				return nil
			})

			p := requestPipelineFn(store, false, redundancy.Level(rLevel))
			ctx := redundancy.SetLevelInContext(cmd.Context(), redundancy.Level(rLevel))
			rootRef, err := p(ctx, reader)
			if err != nil {
				return fmt.Errorf("pipeline: %w", err)
			}
			logger.Info("done", "root", rootRef.String(), "chunks", chunksCount.Load())
			return nil
		},
	}
	c.Flags().String(optionNameInputFile, "", "input file")
	c.Flags().String(optionNameOutputDir, "", "output dir")
	c.Flags().Int(optionNameRedundancyLevel, 0, "redundancy level")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	c.MarkFlagsRequiredTogether(optionNameInputFile, optionNameOutputDir)

	cmd.AddCommand(c)
}
