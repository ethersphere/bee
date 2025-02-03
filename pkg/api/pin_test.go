// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func checkPinHandlers(t *testing.T, client *http.Client, rootHash string, createPin bool) {
	t.Helper()

	const pinsBasePath = "/pins"

	var (
		pinsReferencePath        = pinsBasePath + "/" + rootHash
		pinsInvalidReferencePath = pinsBasePath + "/" + "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2zzz"
		pinsUnknownReferencePath = pinsBasePath + "/" + "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2ccc"
	)

	jsonhttptest.Request(t, client, http.MethodGet, pinsInvalidReferencePath, http.StatusBadRequest)

	jsonhttptest.Request(t, client, http.MethodGet, pinsUnknownReferencePath, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusNotFound),
			Code:    http.StatusNotFound,
		}),
	)

	if createPin {
		jsonhttptest.Request(t, client, http.MethodPost, pinsReferencePath, http.StatusCreated,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusCreated),
				Code:    http.StatusCreated,
			}),
		)
	}

	jsonhttptest.Request(t, client, http.MethodGet, pinsReferencePath, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(struct {
			Reference swarm.Address `json:"reference"`
		}{
			Reference: swarm.MustParseHexAddress(rootHash),
		}),
	)

	jsonhttptest.Request(t, client, http.MethodGet, pinsBasePath, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(struct {
			References []swarm.Address `json:"references"`
		}{
			References: []swarm.Address{swarm.MustParseHexAddress(rootHash)},
		}),
	)

	jsonhttptest.Request(t, client, http.MethodDelete, pinsReferencePath, http.StatusOK)

	jsonhttptest.Request(t, client, http.MethodGet, pinsReferencePath, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusNotFound),
			Code:    http.StatusNotFound,
		}),
	)
}

// nolint:paralleltest
func TestPinHandlers(t *testing.T) {
	var (
		storerMock      = mockstorer.New()
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer: storerMock,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	t.Run("bytes", func(t *testing.T) {
		const rootHash = "838d0a193ecd1152d1bb1432d5ecc02398533b2494889e23b8bd5ace30ac2aeb"
		jsonhttptest.Request(t, client, http.MethodPost, "/bytes", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader("this is a simple text")),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)
		checkPinHandlers(t, client, rootHash, true)
	})

	t.Run("no pins", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/pins", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(struct {
				References []swarm.Address `json:"references"`
			}{
				References: make([]swarm.Address, 0),
			}),
		)
	})

	t.Run("bytes missing", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pins/"+swarm.RandAddress(t).String(), http.StatusNotFound)
	})

	t.Run("bzz", func(t *testing.T) {
		tarReader := tarFiles(t, []f{{
			data: []byte("<h1>Swarm"),
			name: "index.html",
			dir:  "",
		}})
		rootHash := "9e178dbd1ed4b748379e25144e28dfb29c07a4b5114896ef454480115a56b237"
		jsonhttptest.Request(t, client, http.MethodPost, "/bzz", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tarReader),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
			jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "true"),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)
		checkPinHandlers(t, client, rootHash, false)

		header := jsonhttptest.Request(t, client, http.MethodPost, "/bzz?name=somefile.txt", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/plain"),
			jsonhttptest.WithRequestHeader(api.SwarmEncryptHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "true"),
			jsonhttptest.WithRequestBody(strings.NewReader("this is a simple text")),
		)

		rootHash = strings.Trim(header.Get(api.ETagHeader), "\"")
		checkPinHandlers(t, client, rootHash, false)
	})
}

func TestPinHandlersInvalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name      string
		reference string
		want      jsonhttp.StatusResponse
	}{{
		name:      "reference - odd hex string",
		reference: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "reference",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:      "reference - invalid hex character",
		reference: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "reference",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		for _, tc := range tests {
			t.Run(method+" "+tc.name, func(t *testing.T) {
				t.Parallel()

				jsonhttptest.Request(t, client, method, "/pins/"+tc.reference, tc.want.Code,
					jsonhttptest.WithExpectedJSONResponse(tc.want),
				)
			})
		}
	}
}

const pinRef = "620fcd78c7ce54da2d1b7cc2274a02e190cbe8fecbc3bd244690ab6517ce8f39"

func TestIntegrityHandler(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			PinIntegrity: &mockPinIntegrity{
				Store:  inmemstore.New(),
				tester: t,
			},
		})

		endp := "/pins/check?ref=" + pinRef

		// When probe is not set health endpoint should indicate that node is not healthy
		jsonhttptest.Request(t, testServer, http.MethodGet, endp, http.StatusOK, jsonhttptest.WithExpectedResponse(nil))
	})

	t.Run("wrong hash format", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			PinIntegrity: &mockPinIntegrity{
				Store:  inmemstore.New(),
				tester: t,
			},
		})

		endp := "/pins/check?ref=0xbadhash"

		// When probe is not set health endpoint should indicate that node is not healthy
		jsonhttptest.Request(t, testServer, http.MethodGet, endp, http.StatusBadRequest, jsonhttptest.WithExpectedResponse(nil))
	})
}

type mockPinIntegrity struct {
	tester *testing.T
	Store  storage.Store
}

func (p *mockPinIntegrity) Check(ctx context.Context, logger log.Logger, pin string, out chan storer.PinStat) {
	if pin != pinRef {
		p.tester.Fatal("bad pin", pin)
	}
	close(out)
}
