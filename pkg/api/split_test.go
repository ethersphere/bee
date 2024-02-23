// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/log"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	mockstorer "github.com/ethersphere/bee/pkg/storer/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSplit(t *testing.T) {
	t.Parallel()

	var (
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:   mockstorer.New(),
			Logger:   log.Noop,
			Post:     mockpost.New(mockpost.WithAcceptAll()),
			BeeMode:  api.DevMode,
			DebugAPI: true,
		})
	)
	buf := make([]byte, swarm.ChunkSize*2)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	resp := request(t, client, http.MethodPost, "/split", bytes.NewReader(buf), http.StatusOK)
	defer resp.Body.Close()
	var addressCount, chunkCount int
	for {
		addr := make([]byte, swarm.HashSize)
		_, err = resp.Body.Read(addr)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		addressCount++
		ch := make([]byte, swarm.ChunkSize)
		_, err = resp.Body.Read(ch)
		if err != nil {
			t.Fatal(err)
		}
		chunkCount++
	}
	if addressCount != 3 {
		t.Fatalf("number of addresses %d, expected 3", addressCount)
	}
	if addressCount != chunkCount {
		t.Fatalf("number of chunks and addresses do not match")
	}
}
