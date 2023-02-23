// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	mockstorer "github.com/ethersphere/bee/pkg/localstorev2/mock"
	"github.com/ethersphere/bee/pkg/log"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

// nolint:paralleltest
// TestChunkUploadDownload uploads a chunk to an API that verifies the chunk according
// to a given validator, then tries to download the uploaded data.
func TestChunkUploadDownload(t *testing.T) {
	var (
		chunksEndpoint           = "/chunks"
		chunksResource           = func(a swarm.Address) string { return "/chunks/" + a.String() }
		chunk                    = testingc.GenerateTestRandomChunk()
		storerMock               = mockstorer.New()
		client, _, _, chanStorer = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			DirectUpload: true,
		})
	)

	t.Run("empty chunk", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "insufficient data length",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("ok", func(t *testing.T) {
		tag, err := storerMock.NewSession()
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, fmt.Sprintf("%d", tag.TagID)),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		has, err := storerMock.ChunkStore().Has(context.Background(), chunk.Address())
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

	t.Run("direct upload ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		has, err := chanStorer.Has(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk reference: have none; want one")
		}
	})

	// t.Run("pin-invalid-value", func(t *testing.T) {
	// 	jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
	// 		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
	// 		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
	// 		jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
	// 		jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
	// 		jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "invalid-pin"),
	// 	)

	// 	// Also check if the chunk is NOT pinned
	// 	if storerMock.GetModeSet(chunk.Address()) == storage.ModeSetPin {
	// 		t.Fatal("chunk should not be pinned")
	// 	}
	// })
	// t.Run("pin-header-missing", func(t *testing.T) {
	// 	jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
	// 		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
	// 		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
	// 		jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
	// 		jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
	// 	)

	// 	// Also check if the chunk is NOT pinned
	// 	if storerMock.GetModeSet(chunk.Address()) == storage.ModeSetPin {
	// 		t.Fatal("chunk should not be pinned")
	// 	}
	// })
	// t.Run("pin-ok", func(t *testing.T) {
	// 	reference := chunk.Address()
	// 	jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
	// 		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
	// 		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
	// 		jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
	// 		jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: reference}),
	// 		jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "True"),
	// 	)

	// 	has, err := storerMock.ChunkStore().Has(context.Background(), reference)
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	if !has {
	// 		t.Fatal("storer check root chunk reference: have none; want one")
	// 	}

	// 	refs, err := storerMock.Pins()
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	if have, want := len(refs), 1; have != want {
	// 		t.Fatalf("root pin count mismatch: have %d; want %d", have, want)
	// 	}
	// 	if have, want := refs[0], reference; !have.Equal(want) {
	// 		t.Fatalf("root pin reference mismatch: have %q; want %q", have, want)
	// 	}
	// })
}

// nolint:paralleltest
func TestChunkHasHandler(t *testing.T) {
	mockStorer := mockstorer.New()
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockStorer,
	})

	key := swarm.MustParseHexAddress("aabbcc")
	value := []byte("data data data")

	err := mockStorer.Cache().Put(context.Background(), swarm.NewChunk(key, value))
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

	// t.Run("remove-chunk", func(t *testing.T) {
	// 	jsonhttptest.Request(t, testServer, http.MethodDelete, "/chunks/"+key.String(), http.StatusOK,
	// 		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
	// 			Message: http.StatusText(http.StatusOK),
	// 			Code:    http.StatusOK,
	// 		}),
	// 	)
	// 	yes, err := mockStorer.ChunkStore().Has(context.Background(), key)
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	if yes {
	// 		t.Fatalf("The chunk %s is not deleted", key.String())
	// 	}
	// })

	// t.Run("remove-not-present-chunk", func(t *testing.T) {
	// 	notPresentChunkAddress := "deadbeef"
	// 	jsonhttptest.Request(t, testServer, http.MethodDelete, "/chunks/"+notPresentChunkAddress, http.StatusOK,
	// 		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
	// 			Message: http.StatusText(http.StatusOK),
	// 			Code:    http.StatusOK,
	// 		}),
	// 	)
	// 	yes, err := mockStorer.ChunkStore().Has(context.Background(), swarm.NewAddress([]byte(notPresentChunkAddress)))
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	if yes {
	// 		t.Fatalf("The chunk %s is not deleted", notPresentChunkAddress)
	// 	}
	// })
}

func TestChunkHandlersInvalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		address string
		want    jsonhttp.StatusResponse
	}{{
		name:    "address odd hex string",
		address: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "address",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "address invalid hex character",
		address: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "address",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}}

	method := http.MethodGet
	for _, tc := range tests {
		tc := tc
		t.Run(method+" "+tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, method, "/chunks/"+tc.address, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func TestChunkInvalidParams(t *testing.T) {
	t.Parallel()

	var (
		chunksEndpoint = "/chunks"
		chunk          = testingc.GenerateTestRandomChunk()
		storerMock     = mockstorer.New()
		logger         = log.Noop
		existsFn       = func(id []byte) (bool, error) {
			return false, errors.New("error")
		}
	)

	t.Run("batch unusable", func(t *testing.T) {
		t.Parallel()

		clientBatchUnusable, _, _, _ := newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Logger:     logger,
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(),
		})
		jsonhttptest.Request(t, clientBatchUnusable, http.MethodPost, chunksEndpoint, http.StatusUnprocessableEntity,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
		)
	})

	t.Run("batch exists", func(t *testing.T) {
		t.Parallel()

		clientBatchExists, _, _, _ := newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Logger:     logger,
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(mockbatchstore.WithExistsFunc(existsFn)),
		})
		jsonhttptest.Request(t, clientBatchExists, http.MethodPost, chunksEndpoint, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
		)
	})

	t.Run("batch not found", func(t *testing.T) {
		t.Parallel()

		clientBatchNotFound, _, _, _ := newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(),
		})
		jsonhttptest.Request(t, clientBatchNotFound, http.MethodPost, chunksEndpoint, http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
		)
	})
}

// // TestDirectChunkUpload tests that the direct upload endpoint give correct error message in dev mode
func TestChunkDirectUpload(t *testing.T) {
	t.Parallel()
	var (
		chunksEndpoint  = "/chunks"
		chunk           = testingc.GenerateTestRandomChunk()
		storerMock      = mockstorer.New()
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:  storerMock,
			Post:    mockpost.New(mockpost.WithAcceptAll()),
			beeMode: api.DevMode,
		})
	)

	jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusBadRequest,
		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "false"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrUnsupportedDevNodeOperation.Error(),
			Code:    http.StatusBadRequest,
		}),
	)
}
