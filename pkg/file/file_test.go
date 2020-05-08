// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/storage/mock"
	mockbytes "gitlab.com/nolash/go-mockbytes"
)

func TestSplitThenJoin(t *testing.T) {
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	testData, err := g.SequentialBytes(swarm.ChunkSize * (swarm.Branches + 1))
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
	d, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(d, testData) {
		t.Fatal("data mismatch")
	}
}
