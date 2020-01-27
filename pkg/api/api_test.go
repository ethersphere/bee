// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pingpong"
	"resenje.org/web"
)

var _ = testResponseUnmarshal // avoid lint error for unused function

type testServerOptions struct {
	Pingpong pingpong.Interface
}

func newTestServer(t *testing.T, o testServerOptions) (client *http.Client, cleanup func()) {
	s := New(Options{
		Pingpong: o.Pingpong,
		Logger:   logging.New(ioutil.Discard),
	})
	ts := httptest.NewServer(s)
	cleanup = ts.Close

	client = &http.Client{
		Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			u, err := url.Parse(ts.URL + r.URL.String())
			if err != nil {
				return nil, err
			}
			r.URL = u
			return ts.Client().Transport.RoundTrip(r)
		}),
	}
	return client, cleanup
}

func testResponseDirect(t *testing.T, client *http.Client, method, url, body string, responseCode int, response interface{}) {
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

func testResponseUnmarshal(t *testing.T, client *http.Client, method, url, body string, responseCode int, response interface{}) {
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

func request(client *http.Client, method, url, body string, responseCode int) (resp *http.Response, err error) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
