// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/multiformats/go-multiaddr"
	"resenje.org/web"
)

type testServerOptions struct {
	Overlay      swarm.Address
	P2P          p2p.Service
	Pingpong     pingpong.Interface
	Storer       storage.Storer
	TopologyOpts []mock.Option
	Tags         *tags.Tags
}

type testServer struct {
	Client         *http.Client
	TopologyDriver topology.Driver
}

func newTestServer(t *testing.T, o testServerOptions) *testServer {
	topologyDriver := mock.NewTopologyDriver(o.TopologyOpts...)

	s := debugapi.New(debugapi.Options{
		Overlay:        o.Overlay,
		P2P:            o.P2P,
		Pingpong:       o.Pingpong,
		Tags:           o.Tags,
		Logger:         logging.New(ioutil.Discard, 0),
		Storer:         o.Storer,
		TopologyDriver: topologyDriver,
	})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	client := &http.Client{
		Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			u, err := url.Parse(ts.URL + r.URL.String())
			if err != nil {
				return nil, err
			}
			r.URL = u
			return ts.Client().Transport.RoundTrip(r)
		}),
	}
	return &testServer{
		Client: client,
	}
}

func newBZZTestServer(t *testing.T, o testServerOptions) *http.Client {
	s := api.New(api.Options{
		Storer: o.Storer,
		Tags:   o.Tags,
		Logger: logging.New(ioutil.Discard, 0),
	})
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

func mustMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	t.Helper()

	a, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
