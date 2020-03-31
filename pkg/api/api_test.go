// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/storage"
	"resenje.org/web"
)

type testServerOptions struct {
	Pingpong pingpong.Interface
	Storer   storage.Storer
}

func newTestServer(t *testing.T, o testServerOptions) (client *http.Client, cleanup func()) {
	s := api.New(api.Options{
		Pingpong: o.Pingpong,
		Storer:   o.Storer,
		Logger:   logging.New(ioutil.Discard, 0),
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
