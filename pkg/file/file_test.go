// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	test "github.com/ethersphere/bee/v2/pkg/file/testing"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	start = 0
	end   = test.GetVectorCount() - 2
)

// TestSplitThenJoin splits a file with the splitter implementation and
// joins it again with the joiner implementation, verifying that the
// rebuilt data matches the original data that was split.
//
// It uses the same test vectors as the splitter tests to generate the
// necessary data.
func TestSplitThenJoin(t *testing.T) {
	t.Parallel()

	for i := start; i < end; i++ {
		dataLengthStr := strconv.Itoa(i)
		t.Run(dataLengthStr, testSplitThenJoin)
	}
}

func testSplitThenJoin(t *testing.T) {
	t.Parallel()

	var (
		paramstring = strings.Split(t.Name(), "/")
		dataIdx, _  = strconv.ParseInt(paramstring[1], 10, 0)
		store       = inmemchunkstore.New()
		p           = builder.NewPipelineBuilder(context.Background(), store, false, 0)
		data, _     = test.GetVector(t, int(dataIdx))
	)

	// first split
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dataReader := file.NewSimpleReadCloser(data)
	resultAddress, err := builder.FeedPipeline(ctx, p, dataReader)
	if err != nil {
		t.Fatal(err)
	}

	// then join
	r, l, err := joiner.New(ctx, store, store, resultAddress, redundancy.DefaultDownloadLevel)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(len(data)) {
		t.Fatalf("data length return expected %d, got %d", len(data), l)
	}

	// read from joiner
	var resultData []byte
	for i := 0; i < len(data); i += swarm.ChunkSize {
		readData := make([]byte, swarm.ChunkSize)
		_, err := r.Read(readData)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		resultData = append(resultData, readData...)
	}

	// compare result
	if !bytes.Equal(resultData[:len(data)], data) {
		t.Fatalf("data mismatch %d", len(data))
	}
}

// errSplitter is a Splitter that fails without consuming its input, modelling
// an s.Split failure while the copier goroutine is still feeding the pipe.
type errSplitter struct{ err error }

func (s errSplitter) Split(_ context.Context, _ io.ReadCloser, _ int64, _ bool) (swarm.Address, error) {
	return swarm.ZeroAddress, s.err
}

// TestSplitWriteAllSplitError is a regression test for LEAK-01. When s.Split
// fails, the copier goroutine is left blocked writing into the ChunkPipe and
// was never joined, leaking one goroutine and one ChunkPipe per failed upload.
// goleak (see main_test.go) fails the package if the goroutine leaks.
func TestSplitWriteAllSplitError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("split failed")

	// More than one chunk, so the copier blocks writing into the pipe while
	// the (failing) splitter is not reading it.
	data := make([]byte, swarm.ChunkSize*2)

	_, err := file.SplitWriteAll(context.Background(), errSplitter{err: wantErr}, bytes.NewReader(data), int64(len(data)), false)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}
