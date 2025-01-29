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
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	testingpostage "github.com/ethersphere/bee/v2/pkg/postage/testing"
	testingsoc "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// nolint:paralleltest
func TestSOC(t *testing.T) {
	var (
		testData    = []byte("foo")
		socResource = func(owner, id, sig string) string { return fmt.Sprintf("/soc/%s/%s?sig=%s", owner, id, sig) }
		mockStorer  = mockstorer.New()
	)
	t.Run("empty data", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:       mockStorer,
			Post:         newTestPostService(),
			DirectUpload: true,
		})
		jsonhttptest.Request(t, client, http.MethodPost, socResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "bb", "cc"), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "short chunk data",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("signature invalid", func(t *testing.T) {
		s := testingsoc.GenerateMockSOC(t, testData)

		// modify the sign
		sig := make([]byte, swarm.SocSignatureSize)
		copy(sig, s.Signature)
		sig[12] = 0x98
		sig[10] = 0x12

		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:       mockStorer,
			Post:         newTestPostService(),
			DirectUpload: true,
		})
		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(sig)), http.StatusUnauthorized,
			jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid chunk",
				Code:    http.StatusUnauthorized,
			}),
		)
	})

	t.Run("ok", func(t *testing.T) {
		s := testingsoc.GenerateMockSOC(t, testData)
		client, _, _, chanStore := newTestServer(t, testServerOptions{
			Storer:       mockStorer,
			Post:         newTestPostService(),
			DirectUpload: true,
		})

		chanStore.Subscribe(func(ch swarm.Chunk) {
			err := mockStorer.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		})

		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.SocPostResponse{
				Reference: s.Address(),
			}),
		)

		// try to fetch the same chunk
		t.Run("chunks fetch", func(t *testing.T) {
			rsrc := fmt.Sprintf("/chunks/%s", s.Address().String())
			jsonhttptest.Request(t, client, http.MethodGet, rsrc, http.StatusOK,
				jsonhttptest.WithExpectedResponse(s.Chunk().Data()),
				jsonhttptest.WithExpectedContentLength(len(s.Chunk().Data())),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "binary/octet-stream"),
			)
		})

		t.Run("soc fetch", func(t *testing.T) {
			rsrc := fmt.Sprintf("/soc/%s/%s", hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID))
			jsonhttptest.Request(t, client, http.MethodGet, rsrc, http.StatusOK,
				jsonhttptest.WithExpectedResponse(s.WrappedChunk.Data()[swarm.SpanSize:]),
				jsonhttptest.WithExpectedContentLength(len(s.WrappedChunk.Data()[swarm.SpanSize:])),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
			)
		})
	})

	t.Run("postage", func(t *testing.T) {
		s := testingsoc.GenerateMockSOC(t, testData)
		t.Run("err - bad batch", func(t *testing.T) {
			hexbatch := "abcdefgg"
			client, _, _, _ := newTestServer(t, testServerOptions{
				Storer:       mockStorer,
				Post:         newTestPostService(),
				DirectUpload: true,
			})
			jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusBadRequest,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Code:    http.StatusBadRequest,
					Message: "invalid header params",
					Reasons: []jsonhttp.Reason{
						{
							Field: api.SwarmPostageBatchIdHeader,
							Error: api.HexInvalidByteError('g').Error(),
						},
					},
				}))
		})

		t.Run("ok batch", func(t *testing.T) {
			s := testingsoc.GenerateMockSOC(t, testData)
			hexbatch := hex.EncodeToString(batchOk)
			client, _, _, chanStorer := newTestServer(t, testServerOptions{
				Storer:       mockStorer,
				Post:         newTestPostService(),
				DirectUpload: true,
			})
			jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusCreated,
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			)
			err := spinlock.Wait(time.Second, func() bool { return chanStorer.Has(s.Address()) })
			if err != nil {
				t.Fatal(err)
			}
		})

		// TestPreSignedUpload tests that chunk can be uploaded with pre-signed postage stamp
		t.Run("pre-signed upload", func(t *testing.T) {
			t.Parallel()

			var (
				s               = testingsoc.GenerateMockSOC(t, testData)
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
			stamp := testingpostage.MustNewValidStamp(signer, s.Address())
			_ = batchStore.Save(&postage.Batch{
				ID:    stamp.BatchID(),
				Owner: owner.Bytes(),
			})
			stampBytes, _ := stamp.MarshalBinary()

			// read off inserted chunk
			go func() { <-storerMock.PusherFeed() }()

			jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusCreated,
				jsonhttptest.WithRequestHeader(api.SwarmPostageStampHeader, hex.EncodeToString(stampBytes)),
				jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			)
		})

		t.Run("err - batch empty", func(t *testing.T) {
			s := testingsoc.GenerateMockSOC(t, testData)
			hexbatch := hex.EncodeToString(batchEmpty)
			client, _, _, _ := newTestServer(t, testServerOptions{
				Storer:       mockStorer,
				Post:         newTestPostService(),
				DirectUpload: true,
			})
			jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusBadRequest,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			)
		})
	})
}
