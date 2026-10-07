// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The API derives a chunk's address from what it receives, so a client cannot
// submit a chunk under an arbitrary address. A valid case must be accepted and
// stored under its address; for an invalid case, the address the case claims
// must never be stored nor returned as the reference.

// TestChunkUploadValidity posts the chunk data of valid and invalid CACs and
// SOCs to POST /chunks.
func TestChunkUploadValidity(t *testing.T) {
	t.Parallel()

	for _, tc := range testingc.ChunkValidityCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			client, _, _, chanStorer, _ := newTestServer(t, testServerOptions{
				Storer:       mockstorer.New(),
				Post:         mockpost.New(mockpost.WithAcceptAll()),
				DirectUpload: true,
			})

			req, err := http.NewRequest(http.MethodPost, "/chunks", bytes.NewReader(tc.Chunk.Data()))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(api.SwarmPostageBatchIdHeader, batchOkStr)

			assertIngress(t, client, req, chanStorer, tc)
		})
	}
}

// TestSOCUploadValidity posts valid and invalid SOCs to
// POST /soc/{owner}/{id}?sig=. CAC cases do not apply to this endpoint.
func TestSOCUploadValidity(t *testing.T) {
	t.Parallel()

	for _, tc := range testingc.ChunkValidityCases(t) {
		if tc.Owner == nil {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			client, _, _, chanStorer, _ := newTestServer(t, testServerOptions{
				Storer:       mockstorer.New(),
				Post:         newTestPostService(),
				DirectUpload: true,
			})

			data := tc.Chunk.Data()
			id := data[:swarm.HashSize]
			sig := data[swarm.HashSize : swarm.HashSize+swarm.SocSignatureSize]
			wrapped := data[swarm.HashSize+swarm.SocSignatureSize:]

			url := fmt.Sprintf("/soc/%s/%s?sig=%s", hex.EncodeToString(tc.Owner), hex.EncodeToString(id), hex.EncodeToString(sig))
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(wrapped))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(api.SwarmPostageBatchIdHeader, batchOkStr)

			assertIngress(t, client, req, chanStorer, tc)
		})
	}
}

func assertIngress(t *testing.T, client *http.Client, req *http.Request, chanStorer *chanStorer, tc testingc.ChunkCase) {
	t.Helper()

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	var ref swarm.Address
	if resp.StatusCode == http.StatusCreated {
		var r struct {
			Reference swarm.Address `json:"reference"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			t.Fatalf("decode response %q: %v", body, err)
		}
		ref = r.Reference
	}

	addr := tc.Chunk.Address()

	if tc.Valid {
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("got status %d (%s), want %d", resp.StatusCode, body, http.StatusCreated)
		}
		if !ref.Equal(addr) {
			t.Fatalf("got reference %s, want %s", ref, addr)
		}
		if err := spinlock.Wait(time.Second, func() bool { return chanStorer.Has(addr) }); err != nil {
			t.Fatal("valid chunk not stored")
		}
		return
	}

	if ref.Equal(addr) {
		t.Errorf("invalid chunk address %s returned as reference", addr)
	}
	// give the direct upload drain a chance to store anything it received
	time.Sleep(100 * time.Millisecond)
	if chanStorer.Has(addr) {
		t.Errorf("invalid chunk stored under %s (status %d)", addr, resp.StatusCode)
	}
}
