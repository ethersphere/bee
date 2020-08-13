// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/tags"
	"resenje.org/web"
)

type testServerOptions struct {
	Pingpong pingpong.Interface
	Storer   storage.Storer
	Tags     *tags.Tags
	Logger   logging.Logger
}

func newTestServer(t *testing.T, o testServerOptions) *http.Client {
	if o.Logger == nil {
		o.Logger = logging.New(os.Stdout, 5)
	}
	s := api.New(o.Tags, o.Storer, nil, o.Logger, nil)
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	return &http.Client{
		Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			u, err := url.Parse(ts.URL + r.URL.String())
			if err != nil {
				return nil, err
			}
			r.URL = u
			return ts.Client().Transport.RoundTrip(r)
		}),
	}
}
