// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"bufio"
	"bytes"
	"context"
	crand "crypto/rand"
	"io"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/cmd/bee/cmd"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestDBSplitRefs(t *testing.T) {
	t.Parallel()

	s := (rand.Intn(10) + 10) * 1024 // rand between 10 and 20 KB
	buf := make([]byte, s)
	_, err := crand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	inputFileName := path.Join(t.TempDir(), "input")
	err = os.WriteFile(inputFileName, buf, 0644)
	if err != nil {
		t.Fatal(err)
	}

	outputFileName := path.Join(t.TempDir(), "output")

	err = newCommand(t, cmd.WithArgs("split", "refs", "--input-file", inputFileName, "--output-file", outputFileName)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	stat, err := os.Stat(inputFileName)
	if err != nil {
		t.Fatal(err)
	}
	wantHashes := api.CalculateNumberOfChunks(stat.Size(), false) + 1 // +1 for the root hash
	var gotHashes int64

	f, err := os.Open(outputFileName)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		gotHashes++
	}

	if gotHashes != wantHashes {
		t.Fatalf("got %d hashes, want %d", gotHashes, wantHashes)
	}
}

func TestDBSplitChunks(t *testing.T) {
	t.Parallel()

	s := (rand.Intn(10) + 10) * 1024 // rand between 10 and 20 KB
	buf := make([]byte, s)
	_, err := crand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	inputFileName := path.Join(t.TempDir(), "input")
	err = os.WriteFile(inputFileName, buf, 0644)
	if err != nil {
		t.Fatal(err)
	}

	dir := path.Join(t.TempDir(), "chunks")
	err = os.Mkdir(dir, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	err = newCommand(t, cmd.WithArgs("split", "chunks", "--input-file", inputFileName, "--output-dir", dir, "--r-level", "3")).Execute()
	if err != nil {
		t.Fatal(err)
	}

	// split the file manually and compare output with the split commands output.
	putter := &putter{chunks: make(map[string]swarm.Chunk)}
	p := requestPipelineFn(putter, false, redundancy.Level(3))
	ctx := redundancy.SetLevelInContext(context.Background(), redundancy.Level(3))
	_, err = p(ctx, bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != len(putter.chunks) {
		t.Fatal("number of chunks does not match")
	}
	for _, entry := range entries {
		ref := entry.Name()
		if _, ok := putter.chunks[ref]; !ok {
			t.Fatalf("chunk %s not found", ref)
		}
		err, ok := compare(filepath.Join(dir, ref), putter.chunks[ref])
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("chunk %s does not match", ref)
		}
		delete(putter.chunks, ref)
	}

	if len(putter.chunks) != 0 {
		t.Fatalf("want 0 chunks left, got %d", len(putter.chunks))
	}
}

func compare(path string, chunk swarm.Chunk) (error, bool) {
	f, err := os.Open(path)
	if err != nil {
		return err, false
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return err, false
	}

	if !bytes.Equal(b, chunk.Data()) {
		return nil, false
	}

	return nil, true
}

type putter struct {
	chunks map[string]swarm.Chunk
	mu     sync.Mutex
}

func (s *putter) Put(_ context.Context, chunk swarm.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks[chunk.Address().String()] = chunk
	return nil
}

type pipelineFunc func(context.Context, io.Reader) (swarm.Address, error)

func requestPipelineFn(s storage.Putter, encrypt bool, rLevel redundancy.Level) pipelineFunc {
	return func(ctx context.Context, r io.Reader) (swarm.Address, error) {
		pipe := builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
		return builder.FeedPipeline(ctx, pipe, r)
	}
}
