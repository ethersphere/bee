// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"resenje.org/web"
)

type testServerOptions struct {
	P2P p2p.Service
}

func newTestServer(t *testing.T, o testServerOptions) (client *http.Client, cleanup func()) {
	s := New(Options{
		P2P:    o.P2P,
		Logger: logging.New(ioutil.Discard),
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
