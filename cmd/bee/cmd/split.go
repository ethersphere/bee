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

// splitterStore is a store that stores all the split chunk addresses of a file
type splitterStore struct {
	rootHash       string
	chunkAddresses []string
}

func (s *splitterStore) Iterate(ctx context.Context, fn storage.IterateChunkFn) error {
	return nil
}

func (s *splitterStore) Close() error {
	return nil
}

func (s *splitterStore) Get(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
	return nil, nil
}

func (s *splitterStore) Put(ctx context.Context, chunk swarm.Chunk) error {
	s.chunkAddresses = append(s.chunkAddresses, chunk.Address().String())
	return nil
}

func (s *splitterStore) Delete(ctx context.Context, address swarm.Address) error {
	return nil
}

func (s *splitterStore) Has(ctx context.Context, address swarm.Address) (bool, error) {
	return false, nil
}

var _ storage.ChunkStore = (*splitterStore)(nil)

func (c *command) initSplitCmd() error {
	optionNameInputFile := "input-file"
	optionNameOutputFile := "output-file"
	cmd := &cobra.Command{
		Use:   "split",
		Short: "Split a file into a list chunks. The 1st line is the root hash",
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := cmd.Flags().GetString(optionNameVerbosity)
			if err != nil {
				return fmt.Errorf("get verbosity: %w", err)
			}
			v = strings.ToLower(v)
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			inputFileName, err := cmd.Flags().GetString(optionNameInputFile)
			if err != nil {
				return fmt.Errorf("get input file name: %w", err)
			}

			reader, err := os.Open(inputFileName)
			if err != nil {
				return fmt.Errorf("open input file: %w", err)
			}
			defer reader.Close()

			logger.Info("splitting", "file", inputFileName)
			store := new(splitterStore)
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

			outputFileName, err := cmd.Flags().GetString(optionNameOutputFile)
			if err != nil {
				return fmt.Errorf("get output file name: %w", err)
			}

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
			logger.Info("done", "chunks", len(store.chunkAddresses))
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
