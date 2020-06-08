// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package splitter provides implementations of the file.Splitter interface
package splitter_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
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
		c, err = buf.Read(data)
		if c != 2 {
			errC <- fmt.Errorf("read expected 2, got %d", c)
		}
		if err != nil {
			errC <- err
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

	err = buf.Close()
	if err != nil {
		t.Fatal(err)
	}

	timer := time.NewTimer(time.Millisecond)
	select {
		case err = <-errC:
		case <-timer.C:
		t.Fatal("timeout")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnalignedSplit(t *testing.T) {
	var (
		storer storage.Storer = mock.NewStorer()
		chunkBuffer = splitter.NewChunkBuffer()
	)

	// see pkg/file/testing/vector.go
	var (
		dataLen int64 = swarm.ChunkSize*2+32
		expectAddrHex = "61416726988f77b874435bdd89a419edc3861111884fd60e8adf54e2f299efd6"
		g = mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	)
	content, err := g.SequentialBytes(int(dataLen))
	if err != nil {
		t.Fatal(err)
	}

	sp := splitter.NewSimpleSplitter(storer)
	ctx := context.Background()
	doneC := make(chan swarm.Address)
	go func() {
		addr, err := sp.Split(ctx, chunkBuffer, dataLen)
		if err != nil {
			t.Fatal(err)
		}
		doneC <- addr
		close(doneC)
	}()

	contentBuf := bytes.NewReader(content)
	cursor := 0
	writeSizes := []int{swarm.ChunkSize-40, 40+32, swarm.ChunkSize}
	for _, writeSize := range writeSizes {
		data := make([]byte, writeSize)
		_, err = contentBuf.Read(data)
		if err != nil {
			t.Fatal(err)
		}
		c, err := chunkBuffer.Write(data)
		if err != nil {
			t.Fatal(err)
		}
		cursor += c
	}

	err = chunkBuffer.Close()
	if err != nil {
		t.Fatal(err)
	}

	addr := <-doneC
	expectAddr := swarm.MustParseHexAddress(expectAddrHex)
	if !expectAddr.Equal(addr) {
		t.Fatalf("addr mismatch, expected %s, got %s", expectAddr, addr)
	}
}
