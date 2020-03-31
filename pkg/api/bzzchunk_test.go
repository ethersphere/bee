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

	const requestPath = "/bzz-chunk"

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

	validContentBuffer := bytes.NewBuffer(validContent)
	invalidContentBuffer := bytes.NewBuffer(invalidContent)

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
		res := request(t, client, http.MethodPost, requestPath+"/"+validHash.String(), validContentBuffer, 200)
		if res == nil {
			t.Fatal("err nil resposen")
		}
	})

	t.Run("invalid hash", func(t *testing.T) {
		res := request(t, client, http.MethodPost, requestPath+"/"+invalidHash.String(), validContentBuffer, 400)
		if res == nil {
			t.Fatal("err nil resposen")
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		res := request(t, client, http.MethodPost, requestPath+"/"+invalidHash.String(), invalidContentBuffer, 400)
		if res == nil {
			t.Fatal("err nil resposen")
		}
	})
}

func request(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}
