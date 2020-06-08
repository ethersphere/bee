// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package splitter provides implementations of the file.Splitter interface
package splitter_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/logging"
	mockbytes "gitlab.com/nolash/go-mockbytes"
)

func TestChunkBuffer(t *testing.T) {
	buf := splitter.NewChunkBuffer()

	errC := make(chan error)
	go func() {
		data := make([]byte, swarm.ChunkSize)
		c, err := buf.Read(data)
		if err != nil {
			errC <- err
		}
		if c != swarm.ChunkSize {
			errC <- fmt.Errorf("short read %d", c)
		}
		errC <- nil
	}()
	data := [swarm.ChunkSize-2]byte{}
	c, err := buf.Write(data[:])
	if err != nil {
		t.Fatal(err)
	}
	if c != len(data) {
		t.Fatalf("short write")
	}
	c, err = buf.Write(data[:4])
	if err != nil {
		t.Fatal(err)
	}
	if c != 4 {
		t.Fatalf("short write")
	}

	err = <-errC
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnalignedReads(t *testing.T) {
	t.Skip("fix chunk buffer first")
	var (
		storer storage.Storer = mock.NewStorer()
		dataLen int64 = swarm.ChunkSize+42
		logger = logging.New(os.Stderr, 5)
		expectAddrHex = "73759673a52c1f1707cbb61337645f4fcbd209cdc53d7e2cedaaa9f44df61285"
	)

	r, w := io.Pipe()
	sp := splitter.NewSimpleSplitter(storer)
	ctx := context.Background()

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(swarm.ChunkSize+32)
	if err != nil {
		t.Fatal(err)
	}

	doneC := make(chan swarm.Address)
	go func() {
		addr, err := sp.Split(ctx, r, dataLen)
		if err != nil {
			t.Fatal(err)
		}
		doneC <- addr
		close(doneC)
	}()
	contentBuf := bytes.NewReader(content)
	data := make([]byte, swarm.ChunkSize-2)
	_, _ = contentBuf.Read(data)
	logger.Debugf("writing %x", data)
	c, err := w.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	data = make([]byte, 4)
	_, _ = contentBuf.Read(data)
	c, err = w.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	data = make([]byte, 40)
	_, _ = contentBuf.Read(data)
	c, err = w.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	err = w.Close()
	if err != nil {
		t.Fatal(err)
	}
	logger.Debugf("wrote %d", c)
	addr := <-doneC
	expectAddr := swarm.MustParseHexAddress(expectAddrHex)
	if !expectAddr.Equal(addr) {
		t.Fatalf("addr mismatch, expected %s, got %s", expectAddr, addr)
	}
}
