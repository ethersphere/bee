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

	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

// putter is a putter that stores all the split chunk addresses of a file
type putter struct {
	c chan swarm.Chunk
}

func (s *putter) Put(ctx context.Context, chunk swarm.Chunk) error {
	s.c <- chunk
	return nil
}
func newPutter() *putter {
	return &putter{
		c: make(chan swarm.Chunk),
	}
}

var _ storage.Putter = (*putter)(nil)

type pipelineFunc func(context.Context, io.Reader) (swarm.Address, error)

func requestPipelineFn(s storage.Putter, encrypt bool) pipelineFunc {
	return func(ctx context.Context, r io.Reader) (swarm.Address, error) {
		pipe := builder.NewPipelineBuilder(ctx, s, encrypt, 0)
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

	c := &cobra.Command{
		Use:   "refs",
		Short: "Write only the chunk referencs to the output file",
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
			logger.Info("writing output", "file", outputFileName)

			store := newPutter()
			writer, err := os.OpenFile(outputFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("open output file: %w", err)
			}
			defer writer.Close()

			var refs []string
			go func() {
				for chunk := range store.c {
					refs = append(refs, chunk.Address().String())
				}
			}()

			p := requestPipelineFn(store, false)
			rootRef, err := p(context.Background(), reader)
			close(store.c)
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
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")

	c.MarkFlagsRequiredTogether(optionNameInputFile, optionNameOutputFile)

	cmd.AddCommand(c)
}

func splitChunks(cmd *cobra.Command) {
	optionNameInputFile := "input-file"
	optionNameOutputDir := "output-dir"

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
			logger.Info("writing output", "dir", outputDir)

			store := newPutter()
			ctx, cancel := context.WithCancel(context.Background())
			var chunksCount int64
			go func() {
				for chunk := range store.c {
					err := writeChunkToFile(outputDir, chunk)
					if err != nil {
						logger.Error(err, "write chunk")
						cancel()
					}
					chunksCount++
				}
			}()

			p := requestPipelineFn(store, false)
			rootRef, err := p(ctx, reader)
			close(store.c)
			if err != nil {
				return fmt.Errorf("pipeline: %w", err)
			}
			logger.Info("done", "root", rootRef.String(), "chunks", chunksCount)
			return nil
		},
	}
	c.Flags().String(optionNameInputFile, "", "input file")
	c.Flags().String(optionNameOutputDir, "", "output dir")
	c.Flags().String(optionNameVerbosity, "info", "verbosity level")
	c.MarkFlagsRequiredTogether(optionNameInputFile, optionNameOutputDir)

	cmd.AddCommand(c)
}

func writeChunkToFile(outputDir string, chunk swarm.Chunk) error {
	path := filepath.Join(outputDir, chunk.Address().String())
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	defer writer.Close()
	if err != nil {
		return fmt.Errorf("open output file: %w", err)
	}
	_, err = writer.Write(chunk.Data())
	if err != nil {
		return fmt.Errorf("write chunk: %w", err)
	}
	return nil
}
