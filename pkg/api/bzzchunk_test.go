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

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestChunkUploadDownload uploads a chunk to an API that verifies the chunk according
// to a given validator, then tries to download the uploaded data.
func TestChunkUploadDownload(t *testing.T) {
	var (
		resource             = func(addr swarm.Address) string { return "/bzz-chunk/" + addr.String() }
		validHash            = swarm.MustParseHexAddress("aabbcc")
		invalidHash          = swarm.MustParseHexAddress("bbccdd")
		validContent         = []byte("bbaatt")
		invalidContent       = []byte("bbaattss")
		mockValidator        = validator.NewMockValidator(validHash, validContent)
		mockValidatingStorer = mock.NewValidatingStorer(mockValidator)
		client, cleanup      = newTestServer(t, testServerOptions{
			Storer: mockValidatingStorer,
		})
	)
	defer cleanup()

	t.Run("invalid hash", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, client, http.MethodPost, resource(invalidHash), bytes.NewReader(validContent), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "chunk write error",
			Code:    http.StatusBadRequest,
		})

		// make sure chunk is not retrievable
		_ = request(t, client, http.MethodGet, resource(invalidHash), nil, http.StatusNotFound)
	})

	t.Run("invalid content", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, client, http.MethodPost, resource(validHash), bytes.NewReader(invalidContent), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "chunk write error",
			Code:    http.StatusBadRequest,
		})

		// make sure not retrievable
		_ = request(t, client, http.MethodGet, resource(validHash), nil, http.StatusNotFound)
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

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
}

func request(t *testing.T, client *http.Client, method string, resource string, body io.Reader, responseCode int) *http.Response {
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
