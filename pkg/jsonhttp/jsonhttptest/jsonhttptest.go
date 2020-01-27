// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
)

func ResponseDirect(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, response interface{}) {
	t.Helper()

	resp, err := request(client, method, url, body, responseCode)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}

	gotBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}

	got := string(bytes.TrimSpace(gotBytes))
	want := string(wantBytes)

	if got != want {
		t.Errorf("got response %s, want %s", got, want)
	}
}

func ResponseUnmarshal(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, response interface{}) {
	t.Helper()

	resp, err := request(client, method, url, body, responseCode)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatal(err)
	}
}

func request(client *http.Client, method, url string, body io.Reader, responseCode int) (resp *http.Response, err error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}
