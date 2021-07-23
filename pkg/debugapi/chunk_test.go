// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
)

func TestHasChunkHandler(t *testing.T) {
	mockStorer := mock.NewStorer()
	testServer := newTestServer(t, testServerOptions{
		Storer: mockStorer,
	})

	key := swarm.MustParseHexAddress("aabbcc")
	value := []byte("data data data")

	_, err := mockStorer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(key, value))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chunks/"+key.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chunks/abbbbb", http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusNotFound),
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("bad address", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chunks/abcd1100zz", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad address",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("remove-chunk", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodDelete, "/chunks/"+key.String(), http.StatusOK,
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
		jsonhttptest.Request(t, testServer.Client, http.MethodDelete, "/chunks/"+notPresentChunkAddress, http.StatusOK,
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

func TestChunkRelatedAddressesListHandler(t *testing.T) {
	mockStorer := mock.NewStorer()
	traverser := traversal.New(mockStorer)
	testServer := newTestServer(t, testServerOptions{
		Storer:    mockStorer,
		Traverser: traverser,
	})

	ctx := context.Background()
	pipe := builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
	addr, err := builder.FeedPipeline(ctx, pipe, strings.NewReader("hello test world"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("addresses", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chunks/"+addr.String()+"/list", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.ChunkRelatedAddressesListResponse{
				Addresses: []swarm.Address{
					swarm.MustParseHexAddress("e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a"),
				},
			}),
		)
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chunks/abbbbb/list", http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusNotFound),
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("bad address", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chunks/abcd1100zz/list", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad address",
				Code:    http.StatusBadRequest,
			}),
		)
	})
}