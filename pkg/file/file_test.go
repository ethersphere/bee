// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	start = 0
	end   = test.GetVectorCount()
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
		s           = splitter.NewSimpleSplitter(store)
		j           = joiner.NewSimpleJoiner(store)
		data, _     = test.GetVector(t, int(dataIdx))
	)

	// first split
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dataReader := file.NewSimpleReadCloser(data)
	resultAddress, err := s.Split(ctx, dataReader, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	// then join
	r, l, err := j.Join(ctx, resultAddress)
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

// TestJoinReadAll verifies that data in excess of a single chunk is returned
// in its entirety.
func TestJoinReadAll(t *testing.T) {
	var dataLength int64 = swarm.ChunkSize + 2
	j := newMockJoiner(dataLength)
	buf := bytes.NewBuffer(nil)
	err := file.JoinReadAll(j, swarm.ZeroAddress, buf)
	if err != nil {
		t.Fatal(err)
	}
	if dataLength != int64(len(buf.Bytes())) {
		t.Fatalf("expected length %d, got %d", dataLength, len(buf.Bytes()))
	}
}

// mockJoiner is an implementation of file,Joiner that short-circuits that returns
// a mock byte vector of the length given at initialization.
type mockJoiner struct {
	l int64
}

// Join implements file.Joiner.
func (j *mockJoiner) Join(ctx context.Context, address swarm.Address) (dataOut io.ReadCloser, dataLength int64, err error) {
	data := make([]byte, j.l)
	buf := bytes.NewBuffer(data)
	readCloser := ioutil.NopCloser(buf)
	return readCloser, j.l, nil
}

func (j *mockJoiner) Size(ctx context.Context, address swarm.Address) (dataSize int64, err error) {
	return j.l, nil
}

// newMockJoiner creates a new mockJoiner.
func newMockJoiner(l int64) file.Joiner {
	return &mockJoiner{
		l: l,
	}
}
