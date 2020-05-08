// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/storage/mock"
	mockbytes "gitlab.com/nolash/go-mockbytes"
)

func TestSplitThenJoin(t *testing.T) {

	logger := logging.New(os.Stderr, 6)
	chunkCount := swarm.Branches + 2

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	testData, err := g.SequentialBytes(swarm.ChunkSize * chunkCount)
	if err != nil {
		t.Fatal(err)
	}

	store := mock.NewStorer()
	s := splitter.NewSimpleSplitter(store)


	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	testDataReader := file.NewSimpleReadCloser(testData)
	resultAddress, err := s.Split(ctx, testDataReader, int64(len(testData)))
	if err != nil {
		t.Fatal(err)
	}

	j := joiner.NewSimpleJoiner(store)
	r, l, err := j.Join(ctx, resultAddress)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(len(testData)) {
		t.Fatalf("data length return expected %d, got %d", len(testData), l)
	}

	var resultData []byte
	for i := 0; i < chunkCount; i++ {
		readData := make([]byte, swarm.ChunkSize)
		c, err := r.Read(readData)
		if err != nil {
			t.Fatal(err)
		}
		if c < swarm.ChunkSize {
			t.Fatalf("shortread %d", c)
		}
		resultData = append(resultData, readData...)
		logger.Debugf("added data %v..%v", readData[:8], readData[len(readData)-8:])
	}

	if !bytes.Equal(resultData, testData) {
		t.Fatal("data mismatch")
	}
}
