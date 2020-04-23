// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage/mock"
)

func TestBzz(t *testing.T) {
	var (
		resource        = "/bzz/"
		content         = []byte("foo")
		expHash         = "b9d678ef39fa973b430795a1f04e0f2541b47c996fd300552a1e8bfb5824325f"
		mockStorer      = mock.NewStorer()
		client, cleanup = newTestServer(t, testServerOptions{
			Storer: mockStorer,
		})
	)
	defer cleanup()

	t.Run("upload", func(t *testing.T) {
		resp := request(t, client, http.MethodPost, resource, bytes.NewReader(content), http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if string(data) != expHash {
			t.Fatalf("hash mismatch. got %s, want %s", string(data), expHash)
		}
	})

	t.Run("download", func(t *testing.T) {
		resp := request(t, client, http.MethodGet, resource+expHash, nil, http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, content) {
			t.Fatalf("data mismatch. got %s, want %s", string(data), string(content))
		}
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, client, http.MethodGet, resource+"abcd", nil, http.StatusNotFound, jsonhttp.StatusResponse{
			Message: "not found",
			Code:    http.StatusNotFound,
		})
	})
}
