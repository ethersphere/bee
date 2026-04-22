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
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	testingpostage "github.com/ethersphere/bee/v2/pkg/postage/testing"
	testingsoc "github.com/ethersphere/bee/v2/pkg/soc/testing"
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

// TestPreSignedUpload tests that chunk can be uploaded with pre-signed postage stamp
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

// nolint:paralleltest,tparallel
// TestChunkUploadSOC tests that SOC chunks uploaded via POST /chunks are correctly
// identified and stored at their SOC address, not misclassified as CAC chunks.
func TestChunkUploadSOC(t *testing.T) {
	t.Parallel()

	var (
		chunksEndpoint = "/chunks"
		chunksResource = func(a swarm.Address) string { return "/chunks/" + a.String() }
	)

	t.Run("soc upload returns soc address", func(t *testing.T) {
		var (
			mockSOC    = testingsoc.GenerateMockSOC(t, []byte("test payload"))
			socChunk   = mockSOC.Chunk()
			storerMock = mockstorer.New()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: storerMock,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})
		)

		// drain pusher feed
		go func() { <-storerMock.PusherFeed() }()

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(socChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: mockSOC.Address()}),
		)
	})

	t.Run("soc upload chunk is retrievable", func(t *testing.T) {
		var (
			mockSOC    = testingsoc.GenerateMockSOC(t, []byte("retrievable"))
			socChunk   = mockSOC.Chunk()
			storerMock = mockstorer.New()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: storerMock,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})
		)

		// drain pusher feed
		go func() { <-storerMock.PusherFeed() }()

		// upload
		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(socChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: mockSOC.Address()}),
		)

		// retrieve by SOC address
		jsonhttptest.Request(t, client, http.MethodGet, chunksResource(mockSOC.Address()), http.StatusOK,
			jsonhttptest.WithExpectedResponse(socChunk.Data()),
			jsonhttptest.WithExpectedContentLength(len(socChunk.Data())),
		)
	})

	t.Run("soc direct upload", func(t *testing.T) {
		var (
			mockSOC    = testingsoc.GenerateMockSOC(t, []byte("direct"))
			socChunk   = mockSOC.Chunk()
			storerMock = mockstorer.New()
			client, _, _, chanStorer = newTestServer(t, testServerOptions{
				Storer:       storerMock,
				Post:         mockpost.New(mockpost.WithAcceptAll()),
				DirectUpload: true,
			})
		)

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(socChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: mockSOC.Address()}),
		)

		time.Sleep(time.Millisecond * 100)
		err := spinlock.Wait(time.Second, func() bool { return chanStorer.Has(mockSOC.Address()) })
		if err != nil {
			t.Fatal("soc chunk not found at soc address in direct upload channel")
		}
	})

	t.Run("cac upload still works", func(t *testing.T) {
		var (
			chunk      = testingc.GenerateTestRandomChunk()
			storerMock = mockstorer.New()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: storerMock,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})
		)

		go func() { <-storerMock.PusherFeed() }()

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)
	})

	t.Run("cac upload small payload", func(t *testing.T) {
		// Minimal CAC: span (8 bytes) + 1 byte payload = 9 bytes total
		var (
			cacChunk, _ = cac.New([]byte{0x01})
			storerMock  = mockstorer.New()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: storerMock,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})
		)

		go func() { <-storerMock.PusherFeed() }()

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(cacChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: cacChunk.Address()}),
		)
	})

	t.Run("cac upload max payload", func(t *testing.T) {
		// Maximum CAC: span (8 bytes) + 4096 byte payload = 4104 bytes total
		var (
			payload     = make([]byte, swarm.ChunkSize)
			cacChunk, _ = cac.New(payload)
			storerMock  = mockstorer.New()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: storerMock,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})
		)

		go func() { <-storerMock.PusherFeed() }()

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(cacChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: cacChunk.Address()}),
		)
	})

	t.Run("data too short", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer: mockstorer.New(),
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader([]byte{0x01, 0x02, 0x03})),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "insufficient data length",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("invalid soc falls back to cac", func(t *testing.T) {
		// Data that is >= SocMinChunkSize (105 bytes) but not a valid SOC
		// (random data won't have valid ECDSA signature). Should fall back to CAC.
		var (
			payload     = make([]byte, swarm.SocMinChunkSize-swarm.SpanSize) // 97 bytes payload
			cacChunk, _ = cac.New(payload)                                    // 105 bytes total (span + 97)
			storerMock  = mockstorer.New()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: storerMock,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})
		)

		go func() { <-storerMock.PusherFeed() }()

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(cacChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: cacChunk.Address()}),
		)
	})

	t.Run("soc with pre-signed stamp", func(t *testing.T) {
		var (
			mockSOC    = testingsoc.GenerateMockSOC(t, []byte("stamped"))
			socChunk   = mockSOC.Chunk()
			storerMock = mockstorer.New()
			batchStore = mockbatchstore.New()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer:     storerMock,
				BatchStore: batchStore,
			})
		)

		key, _ := crypto.GenerateSecp256k1Key()
		signer := crypto.NewDefaultSigner(key)
		owner, _ := signer.EthereumAddress()
		stamp := testingpostage.MustNewValidStamp(signer, mockSOC.Address())
		_ = batchStore.Save(&postage.Batch{
			ID:    stamp.BatchID(),
			Owner: owner.Bytes(),
		})
		stampBytes, _ := stamp.MarshalBinary()

		go func() { <-storerMock.PusherFeed() }()

		jsonhttptest.Request(t, client, http.MethodPost, chunksEndpoint, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageStampHeader, hex.EncodeToString(stampBytes)),
			jsonhttptest.WithRequestBody(bytes.NewReader(socChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: mockSOC.Address()}),
		)
	})
}
