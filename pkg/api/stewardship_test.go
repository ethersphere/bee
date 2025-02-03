// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/steward"
	"github.com/ethersphere/bee/v2/pkg/steward/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"gitlab.com/nolash/go-mockbytes"
)

// nolint:paralleltest
func TestStewardship(t *testing.T) {
	var (
		logger      = log.Noop
		stewardMock = &mock.Steward{}
		storer      = mockstorer.New()
		addr        = swarm.NewAddress([]byte{31: 128})
	)
	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer:  storer,
		Logger:  logger,
		Steward: stewardMock,
		Post:    mockpost.New(mockpost.WithAcceptAll()),
	})

	t.Run("re-upload", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPut, "/v1/stewardship/"+addr.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
			jsonhttptest.WithRequestHeader("Swarm-Postage-Batch-Id", "aa"),
		)
		if !stewardMock.LastAddress().Equal(addr) {
			t.Fatalf("\nhave address: %q\nwant address: %q", stewardMock.LastAddress().String(), addr.String())
		}
	})

	t.Run("is-retrievable", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/v1/stewardship/"+addr.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.IsRetrievableResponse{IsRetrievable: true}),
		)
		jsonhttptest.Request(t, client, http.MethodGet, "/v1/stewardship/"+hex.EncodeToString([]byte{}), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: http.StatusText(http.StatusNotFound),
			}),
		)
	})
}

type localRetriever struct {
	getter storage.Getter
}

func (lr *localRetriever) RetrieveChunk(ctx context.Context, addr, sourceAddr swarm.Address) (chunk swarm.Chunk, err error) {
	ch, err := lr.getter.Get(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("retrieve chunk %s: %w", addr, err)
	}
	return ch, nil
}

func TestStewardshipWithRedundancy(t *testing.T) {
	t.Parallel()

	var (
		storerMock      = mockstorer.New()
		localRetrieval  = &localRetriever{getter: storerMock.ChunkStore()}
		s               = steward.New(storerMock, localRetrieval, storerMock.Cache())
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:  storerMock,
			Logger:  log.Noop,
			Steward: s,
			Post:    mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(512000) // 500KB
	if err != nil {
		t.Fatal(err)
	}

	for _, l := range []redundancy.Level{redundancy.NONE, redundancy.MEDIUM, redundancy.STRONG, redundancy.INSANE, redundancy.PARANOID} {
		t.Run(fmt.Sprintf("rLevel-%d", l), func(t *testing.T) {
			res := new(api.BytesPostResponse)
			jsonhttptest.Request(t, client, http.MethodPost, "/bytes", http.StatusCreated,
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
				jsonhttptest.WithRequestHeader(api.SwarmRedundancyLevelHeader, strconv.Itoa(int(l))),
				jsonhttptest.WithRequestBody(bytes.NewReader(content)),
				jsonhttptest.WithUnmarshalJSONResponse(res),
			)

			time.Sleep(2 * time.Second)
			jsonhttptest.Request(t, client, http.MethodGet, "/stewardship/"+res.Reference.String(), http.StatusOK,
				jsonhttptest.WithExpectedJSONResponse(api.IsRetrievableResponse{IsRetrievable: true}),
			)
		})
	}
}

func TestStewardshipInvalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
	})

	tests := []struct {
		name    string
		address string
		want    jsonhttp.StatusResponse
	}{{
		name:    "address - odd hex string",
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
		name:    "address - invalid hex character",
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

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, tc := range tests {
			t.Run(method+" "+tc.name, func(t *testing.T) {
				t.Parallel()

				jsonhttptest.Request(t, client, method, "/stewardship/"+tc.address, tc.want.Code,
					jsonhttptest.WithExpectedJSONResponse(tc.want),
				)
			})
		}
	}

	t.Run("batch with id not found", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, client, http.MethodPut, "/stewardship/1234", http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, "1234"),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: "batch with id not found",
			}),
		)
	})
	t.Run("invalid batch id", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, client, http.MethodPut, "/stewardship/1234", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, "1234G"),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid header params",
				Reasons: []jsonhttp.Reason{
					{
						Field: api.SwarmPostageBatchIdHeader,
						Error: api.HexInvalidByteError('G').Error(),
					},
				},
			}),
		)
	})
}
