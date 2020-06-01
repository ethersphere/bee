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

	resp := request(t, client, method, url, body, responseCode, nil)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	want, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("got response %s, want %s", string(got), string(want))
	}
}


func ResponseDirectSendHeadersAndReceiveHeaders(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int,
	response interface{}, headers http.Header) http.Header {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, headers)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	want, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("got response %s, want %s", string(got), string(want))
	}

	return resp.Header
}

// ResponseDirectWithJson checks for responses in json format. It is useful in cases where the response is json.
func ResponseDirectWithJson(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, response interface{}) {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, nil)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	want, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}
	var wantJson jsonhttp.StatusResponse
	err = json.Unmarshal(want, &wantJson)
	if err != nil {
		t.Error(err)
	}
	wantString := "[" + wantJson.Message + "]"

	if wantString != string(got) {
		t.Errorf("got response %s, want %s", string(got), wantString)
	}
}

func ResponseDirectWithHeaders(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int,
	response interface{}, headers http.Header) {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, headers)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	want, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("got response %s, want %s", string(got), string(want))
	}
}

func ResponseUnmarshal(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, response interface{}) {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, nil)
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
}

func request(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, headers http.Header) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = headers
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}
	return resp
}
