// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	testingpostage "github.com/ethersphere/bee/v2/pkg/postage/testing"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// nolint:paralleltest,tparallel
// TestChunkUploadDownload uploads a chunk to an API that verifies the chunk according
// to a given validator, then tries to download the uploaded data.
func TestChunkUploadDownload(t *testing.T) {
	t.Parallel()

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
		jsonhttptest.Request(t, client, http.MethodGet, chunksResource(chunk.Address()), http.StatusOK,
			jsonhttptest.WithExpectedResponse(chunk.Data()),
			jsonhttptest.WithExpectedContentLength(len(chunk.Data())),
		)
	})

	t.Run("direct upload ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		time.Sleep(time.Millisecond * 100)
		err := spinlock.Wait(time.Second, func() bool { return chanStorer.Has(chunk.Address()) })
		if err != nil {
			t.Fatal(err)
		}
	})
}

// nolint:paralleltest,tparallel
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
			jsonhttptest.WithNoResponseBody(),
		)

		jsonhttptest.Request(t, testServer, http.MethodGet, "/chunks/"+key.String(), http.StatusOK,
			jsonhttptest.WithExpectedResponse(value),
			jsonhttptest.WithExpectedContentLength(len(value)),
		)
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.Request(t, testServer, http.MethodHead, "/chunks/abbbbb", http.StatusNotFound,
			jsonhttptest.WithNoResponseBody())
	})

	t.Run("bad address", func(t *testing.T) {
		jsonhttptest.Request(t, testServer, http.MethodHead, "/chunks/abcd1100zz", http.StatusBadRequest,
			jsonhttptest.WithNoResponseBody())
	})
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
			BeeMode: api.DevMode,
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

// // TestPreSignedUpload tests that chunk can be uploaded with pre-signed postage stamp
func TestPreSignedUpload(t *testing.T) {
	t.Parallel()

	var (
		chunksEndpoint  = "/chunks"
		chunk           = testingc.GenerateTestRandomChunk()
		storerMock      = mockstorer.New()
		batchStore      = mockbatchstore.New()
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:     storerMock,
			BatchStore: batchStore,
		})
	)

	// generate random postage batch and stamp
	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)
	owner, _ := signer.EthereumAddress()
	stamp := testingpostage.MustNewValidStamp(signer, chunk.Address())
	_ = batchStore.Save(&postage.Batch{
		ID:    stamp.BatchID(),
		Owner: owner.Bytes(),
	})
	stampBytes, _ := stamp.MarshalBinary()

	// read off inserted chunk
	go func() { <-storerMock.PusherFeed() }()

	jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
		jsonhttptest.WithRequestHeader(api.SwarmPostageStampHeader, hex.EncodeToString(stampBytes)),
		jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
	)
}
