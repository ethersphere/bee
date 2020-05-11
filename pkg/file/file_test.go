// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	start = 0
	end   = test.VectorCount
)

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
		logger      = logging.New(os.Stderr, 6)
		store       = mock.NewStorer()
		s           = splitter.NewSimpleSplitter(store)
		j           = joiner.NewSimpleJoiner(store)
		data, _     = test.GetVector(t, int(dataIdx))
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	dataReader := file.NewSimpleReadCloser(data)
	resultAddress, err := s.Split(ctx, dataReader, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	r, l, err := j.Join(ctx, resultAddress)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(len(data)) {
		t.Fatalf("data length return expected %d, got %d", len(data), l)
	}

	var resultData []byte
	chunkCount := (len(data) / swarm.ChunkSize) + 1
	for i := 0; i < chunkCount; i++ {
		readData := make([]byte, swarm.ChunkSize)
		_, err := r.Read(readData)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		resultData = append(resultData, readData...)

		logger.Debugf("added %d/%d data %v..%v", len(resultData), len(data), readData[:8], readData[len(readData)-8:])
	}

	if !bytes.Equal(resultData[:len(data)], data) {
		t.Fatalf("data mismatch %d", len(data))
	}
	t.Logf("data ok %d", len(data))
}
