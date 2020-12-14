// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
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
	for i := start; i < end; i++ {
		dataLengthStr := strconv.Itoa(i)
		t.Run(dataLengthStr, testSplitThenJoin)
	}
}

func testSplitThenJoin(t *testing.T) {
	var (
		paramstring = strings.Split(t.Name(), "/")
		dataIdx, _  = strconv.ParseInt(paramstring[1], 10, 0)
		store       = mock.NewStorer()
		p           = builder.NewPipelineBuilder(context.Background(), store, storage.ModePutUpload, false, nil)
		data, _     = test.GetVector(t, int(dataIdx))
	)

	// first split
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dataReader := file.NewSimpleReadCloser(data)
	resultAddress, err := builder.FeedPipeline(ctx, p, dataReader, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	// then join
	r, l, err := joiner.New(ctx, store, resultAddress)
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
			if err == io.EOF {
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
