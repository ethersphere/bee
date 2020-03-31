// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"io"
	"net/http"
	"testing"
)

func TestChunkUpload(t *testing.T) {
	// define a mock chunk data, i.e. addr abcd chunk data efgh
	// define some storage service with a validator, the validator is a mock validator
	// that checks that if the data is efgh and hash is abcd then it is valid

	// upload the chunk, "store" it to the mock storage in case it is valid
	// return an http error Bad request in case it isn't

	// download the chunk in both cases, in the first one - make sure it is retrievable
	// in the second - that it isnt
}

func request(t *testing.T, method, url string, body io.Reader, responseCode int) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}
	return resp
}
