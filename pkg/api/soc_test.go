// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/soc"
	testingsoc "github.com/ethersphere/bee/pkg/soc/testing"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestSoc(t *testing.T) {
	var (
		testData       = []byte("foo")
		socResource    = func(owner, id, sig string) string { return fmt.Sprintf("/soc/%s/%s?sig=%s", owner, id, sig) }
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		tag            = tags.NewTags(mockStatestore, logger)
		mockStorer     = mock.NewStorer()
		client, _, _   = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tag,
		})
	)
	t.Run("cmpty data", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "bb", "cc"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "short chunk data",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed id", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "bbzz", "cc"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad id",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed owner", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("xyz", "aa", "bb"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad owner",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed signature", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, socResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "aa", "badsig"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad signature",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("signature invalid", func(t *testing.T) {
		s := testingsoc.GenerateMockSoc(t, testData)

		// modify the sign
		sig := make([]byte, soc.SignatureSize)
		copy(sig, s.Signature)
		sig[12] = 0x98
		sig[10] = 0x12

		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(sig)), http.StatusUnauthorized,
			jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid chunk",
				Code:    http.StatusUnauthorized,
			}),
		)
	})

	t.Run("ok", func(t *testing.T) {
		s := testingsoc.GenerateMockSoc(t, testData)

		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusCreated,
			jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.SocPostResponse{
				Reference: s.Address(),
			}),
		)

		// try to fetch the same chunk
		rsrc := fmt.Sprintf("/chunks/" + s.Address().String())
		resp := request(t, client, http.MethodGet, rsrc, nil, http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(s.Chunk().Data(), data) {
			t.Fatal("data retrieved doesn't match uploaded content")
		}
	})

	t.Run("already exists", func(t *testing.T) {
		s := testingsoc.GenerateMockSoc(t, testData)

		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusCreated,
			jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.SocPostResponse{
				Reference: s.Address(),
			}),
		)
		jsonhttptest.Request(t, client, http.MethodPost, socResource(hex.EncodeToString(s.Owner), hex.EncodeToString(s.ID), hex.EncodeToString(s.Signature)), http.StatusConflict,
			jsonhttptest.WithRequestBody(bytes.NewReader(s.WrappedChunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(
				jsonhttp.StatusResponse{
					Message: "chunk already exists",
					Code:    http.StatusConflict,
				}),
		)
	})
}
