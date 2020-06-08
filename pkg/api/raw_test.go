// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
       "bytes"
       "io/ioutil"
       "net/http"
       "os"
       "testing"

       "github.com/ethersphere/bee/pkg/api"
       "github.com/ethersphere/bee/pkg/jsonhttp"
       "github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
       "github.com/ethersphere/bee/pkg/storage/mock"
       "github.com/ethersphere/bee/pkg/swarm"
       "github.com/ethersphere/bee/pkg/tags"
       "github.com/ethersphere/bee/pkg/logging"
	mockbytes "gitlab.com/nolash/go-mockbytes"
)

// TestRaw tests that the data upload api responds as expected when uploading,
// downloading and requesting a resource that cannot be found.
func TestRaw(t *testing.T) {
	var (
	       resource   = "/bzz-raw"
		expHash = "29a5fb121ce96194ba8b7b823a1f9c6af87e1791f824940a53b5a7efe3f790d9"
	       mockStorer = mock.NewStorer()
	       client     = newTestServer(t, testServerOptions{
		       Storer: mockStorer,
		       Tags: tags.NewTags(),
		       Logger: logging.New(os.Stderr, 5),
	       })
	)
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(swarm.ChunkSize*2)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("upload", func(t *testing.T) {
	       jsonhttptest.ResponseDirect(t, client, http.MethodPost, resource, bytes.NewReader(content), http.StatusOK, api.RawPostResponse{
		       Hash: swarm.MustParseHexAddress(expHash),
	       })
	})

	t.Run("download", func(t *testing.T) {
	       resp := request(t, client, http.MethodGet, resource+"/"+expHash, nil, http.StatusOK)
	       data, err := ioutil.ReadAll(resp.Body)
	       if err != nil {
		       t.Fatal(err)
	       }

	       if !bytes.Equal(data, content) {
		       t.Fatalf("data mismatch. got %s, want %s", string(data), string(content))
	       }
	})

	t.Run("not found", func(t *testing.T) {
	       jsonhttptest.ResponseDirect(t, client, http.MethodGet, resource+"/abcd", nil, http.StatusNotFound, jsonhttp.StatusResponse{
		       Message: "not found",
		       Code:    http.StatusNotFound,
	       })
	})
}
