// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/disk"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage/mem"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestChunkUpload uploads a chunk to an API that verifies the chunk according
// to a given validator, then tries to download the uploaded data.
func TestChunkUploadDownload(t *testing.T) {
	resource := func(addr swarm.Address) string {
		return "/bzz-chunk/" + addr.String()
	}

	validHash, err := swarm.ParseHexAddress("aabbcc")
	if err != nil {
		t.Fatal(err)
	}

	invalidHash, err := swarm.ParseHexAddress("bbccdd")
	if err != nil {
		t.Fatal(err)
	}

	validContent := []byte("bbaatt")
	invalidContent := []byte("bbaattss")

	validatorF := func(ch swarm.Chunk) bool {
		if !ch.Address().Equal(validHash) {
			return false

		}
		if !bytes.Equal(ch.Data().Bytes(), validContent) {
			return false
		}
		return true
	}

	mockValidatingStorer, err := mem.NewMemStorer(validatorF)
	if err != nil {
		t.Fatal(err)
	}
	client, cleanup := newTestServer(t, testServerOptions{
		Storer: mockValidatingStorer,
	})
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


// TestChunkUploadDownloadWithPersistance uploads a chunk to an API that verifies the chunk according
// to the chunk validator, restarts the DB and then tries to download and check for correctness.
func TestChunkUploadDownloadWithPersistance(t *testing.T) {
	resource := func(addr swarm.Address) string {
		return "/bzz-chunk/" + addr.String()
	}

	// generate valid and invalid data to test.
	hexStr, err := hexutil.Decode("0xaaaac30623a1a20c48c473e55c8456944772b477")
	if err != nil {
		t.Fatalf("%v", err)
	}
	validHash := swarm.NewAddress(hexStr)
	validContent := []byte("bbaatt")

	invalidHash, err := swarm.ParseHexAddress("aabbcc")
	if err != nil {
		t.Fatal(err)
	}
	invalidContent := make([]byte, swarm.DefaultChunkSize + 1)
	rand.Read(invalidContent)

	// create the disk store with the chunk validator.
	dir, err := ioutil.TempDir("", "disk-test")
	if err != nil {
		t.Fatal(err)
	}

	diskValidatingStorer, err := disk.NewDiskStorer(dir, storage.ValidateContentChunk)
	if err != nil {
		t.Fatal(err)
	}
	client, cleanup := newTestServer(t, testServerOptions{
		Storer: diskValidatingStorer,
	})


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

		// Close the server and the DB
		ctx := context.Background()
		err = diskValidatingStorer.Close(ctx)
		if err != nil {
			t.Fatal(err)
		}
		cleanup()


		// Open the diskstore and create a new server to retrieve the contents
		diskValidatingStorer, err := disk.NewDiskStorer(dir, storage.ValidateContentChunk)
		if err != nil {
			t.Fatal(err)
		}
		client, cleanup = newTestServer(t, testServerOptions{
			Storer: diskValidatingStorer,
		})
		defer cleanup()

		// try to fetch the same chunk after a reboot
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