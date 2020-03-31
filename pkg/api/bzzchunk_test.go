// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestChunkUpload(t *testing.T) {
	// define a mock chunk data, i.e. addr abcd chunk data efgh
	// define some storage service with a validator, the validator is a mock validator
	// that checks that if the data is efgh and hash is abcd then it is valid

	// upload the chunk, "store" it to the mock storage in case it is valid
	// return an http error Bad request in case it isn't

	// download the chunk in both cases, in the first one - make sure it is retrievable
	// in the second - that it isnt

	const requestPath = "/bzz-chunk/"

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

	validatorF := func(addr swarm.Address, data []byte) bool {
		if !addr.Equal(validHash) {
			return false

		}
		if !bytes.Equal(data, validContent) {
			return false
		}
		return true
	}

	mockValidatingStorer := mock.NewValidatingStorer(validatorF)
	client, cleanup := newTestServer(t, testServerOptions{
		Storer: mockValidatingStorer,
	})
	defer cleanup()

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, client, http.MethodPost, requestPath+validHash.String(), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})
	})
	t.Run("invalid hash", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, client, http.MethodPost, requestPath+invalidHash.String(), bytes.NewReader(validContent), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "chunk write error",
			Code:    http.StatusBadRequest,
		})
	})
	t.Run("invalid content", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, client, http.MethodPost, requestPath+validHash.String(), bytes.NewReader(invalidContent), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "chunk write error",
			Code:    http.StatusBadRequest,
		})
	})
}
