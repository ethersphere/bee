// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"

	"github.com/ethersphere/bee/pkg/tags"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestChunkUploadDownload uploads a chunk to an API that verifies the chunk according
// to a given validator, then tries to download the uploaded data.
func TestChunkUploadDownload(t *testing.T) {

	var (
		targets              = "0x222"
		resource             = func(addr swarm.Address) string { return "/chunks/" + addr.String() }
		resourceTargets      = func(addr swarm.Address) string { return "/chunks/" + addr.String() + "?targets=" + targets }
		validHash            = swarm.MustParseHexAddress("aabbcc")
		invalidHash          = swarm.MustParseHexAddress("bbccdd")
		validContent         = []byte("bbaatt")
		invalidContent       = []byte("bbaattss")
		mockValidator        = validator.NewMockValidator(validHash, validContent)
		mockStatestore       = statestore.NewStateStore()
		logger               = logging.New(ioutil.Discard, 0)
		tag                  = tags.NewTags(mockStatestore, logger)
		mockValidatingStorer = mock.NewStorer(mock.WithValidator(mockValidator))
		client, _, _         = newTestServer(t, testServerOptions{
			Storer: mockValidatingStorer,
			Tags:   tag,
		})
	)

	t.Run("invalid hash", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource(invalidHash), http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(validContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "chunk write error",
				Code:    http.StatusBadRequest,
			}),
		)

		// make sure chunk is not retrievable
		_ = request(t, client, http.MethodGet, resource(invalidHash), nil, http.StatusNotFound)
	})

	t.Run("invalid content", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource(invalidHash), http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(invalidContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "chunk write error",
				Code:    http.StatusBadRequest,
			}),
		)

		// make sure not retrievable
		_ = request(t, client, http.MethodGet, resource(validHash), nil, http.StatusNotFound)
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource(validHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(validContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// try to fetch the same chunk
		resp := request(t, client, http.MethodGet, resource(validHash), nil, http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(validContent, data) {
			t.Fatal("data retrieved doesnt match uploaded content")
		}
	})

	t.Run("pin-invalid-value", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource(validHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(validContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "invalid-pin"),
		)

		// Also check if the chunk is NOT pinned
		if mockValidatingStorer.GetModeSet(validHash) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-header-missing", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource(validHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(validContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		// Also check if the chunk is NOT pinned
		if mockValidatingStorer.GetModeSet(validHash) == storage.ModeSetPin {
			t.Fatal("chunk should not be pinned")
		}
	})
	t.Run("pin-ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource(validHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(validContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "True"),
		)

		// Also check if the chunk is pinned
		if mockValidatingStorer.GetModePut(validHash) != storage.ModePutUploadPin {
			t.Fatal("chunk is not pinned")
		}

	})
	t.Run("retrieve-targets", func(t *testing.T) {
		resp := request(t, client, http.MethodGet, resourceTargets(validHash), nil, http.StatusOK)

		// Check if the target is obtained correctly
		if resp.Header.Get(api.TargetsRecoveryHeader) != targets {
			t.Fatalf("targets mismatch. got %s, want %s", resp.Header.Get(api.TargetsRecoveryHeader), targets)
		}
	})
}

func request(t *testing.T, client *http.Client, method, resource string, body io.Reader, responseCode int) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, resource, body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != responseCode {
		t.Fatalf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}
	return resp
}
