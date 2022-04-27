// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	pinning "github.com/ethersphere/bee/pkg/pinning/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"

	"github.com/ethersphere/bee/pkg/tags"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestChunkUploadDownload uploads a chunk to an API that verifies the chunk according
// to a given validator, then tries to download the uploaded data.
func TestChunkUploadDownload(t *testing.T) {
	var (
		chunksEndpoint  = "/chunks"
		chunksResource  = func(a swarm.Address) string { return "/chunks/" + a.String() }
		chunk           = testingc.GenerateTestRandomChunk()
		statestoreMock  = statestore.NewStateStore()
		logger          = logging.New(io.Discard, 0)
		tag             = tags.NewTags(statestoreMock, logger)
		storerMock      = mock.NewStorer()
		pinningMock     = pinning.NewServiceMock()
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:  storerMock,
			Pinning: pinningMock,
			Tags:    tag,
			Post:    mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	t.Run("empty chunk", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "data length",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("ok", func(t *testing.T) {
		reference := chunk.Address()
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: reference}),
		)

		has, err := storerMock.Has(context.Background(), reference)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk reference: have none; want one")
		}

		// try to fetch the same chunk
		endpoint := chunksResource(chunk.Address())
		resp := request(t, client, http.MethodGet, endpoint, nil, http.StatusOK)
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(chunk.Data(), data) {
			t.Fatal("data retrieved doesnt match uploaded content")
		}
	})

	t.Run("pin-invalid-value", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "invalid-pin"),
		)

		// Also check if the chunk is NOT pinned
		if storerMock.GetModeSet(chunk.Address()) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-header-missing", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		// Also check if the chunk is NOT pinned
		if storerMock.GetModeSet(chunk.Address()) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-ok", func(t *testing.T) {
		reference := chunk.Address()
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: reference}),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "True"),
		)

		has, err := storerMock.Has(context.Background(), reference)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk reference: have none; want one")
		}

		refs, err := pinningMock.Pins()
		if err != nil {
			t.Fatal(err)
		}
		if have, want := len(refs), 1; have != want {
			t.Fatalf("root pin count mismatch: have %d; want %d", have, want)
		}
		if have, want := refs[0], reference; !have.Equal(want) {
			t.Fatalf("root pin reference mismatch: have %q; want %q", have, want)
		}
	})
}

func TestHasChunkHandler(t *testing.T) {
	mockStorer := mock.NewStorer()
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockStorer,
	})

	key := swarm.MustParseHexAddress("aabbcc")
	value := []byte("data data data")

	_, err := mockStorer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(key, value))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, testServer, http.MethodHead, "/chunks/"+key.String(), http.StatusOK,
			jsonhttptest.WithNoResponseBody())
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.Request(t, testServer, http.MethodHead, "/chunks/abbbbb", http.StatusNotFound,
			jsonhttptest.WithNoResponseBody())
	})

	t.Run("bad address", func(t *testing.T) {
		jsonhttptest.Request(t, testServer, http.MethodHead, "/chunks/abcd1100zz", http.StatusBadRequest,
			jsonhttptest.WithNoResponseBody())
	})

	t.Run("remove-chunk", func(t *testing.T) {
		jsonhttptest.Request(t, testServer, http.MethodDelete, "/chunks/"+key.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)
		yes, err := mockStorer.Has(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if yes {
			t.Fatalf("The chunk %s is not deleted", key.String())
		}
	})

	t.Run("remove-not-present-chunk", func(t *testing.T) {
		notPresentChunkAddress := "deadbeef"
		jsonhttptest.Request(t, testServer, http.MethodDelete, "/chunks/"+notPresentChunkAddress, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)
		yes, err := mockStorer.Has(context.Background(), swarm.NewAddress([]byte(notPresentChunkAddress)))
		if err != nil {
			t.Fatal(err)
		}
		if yes {
			t.Fatalf("The chunk %s is not deleted", notPresentChunkAddress)
		}
	})
}
