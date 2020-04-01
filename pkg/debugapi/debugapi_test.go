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

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/addressbook/inmem"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/multiformats/go-multiaddr"
	"resenje.org/web"
)

type testServerOptions struct {
	P2P p2p.Service
}

type testServer struct {
	Client         *http.Client
	Addressbook    addressbook.GetPutter
	TopologyDriver *mock.TopologyDriver
	Cleanup        func()
}

func newTestServer(t *testing.T, o testServerOptions) *testServer {
	addressbook := inmem.New()
	topologyDriver := mock.NewTopologyDriver()

	s := debugapi.New(debugapi.Options{
		P2P:            o.P2P,
		Logger:         logging.New(ioutil.Discard, 0),
		Addressbook:    addressbook,
		TopologyDriver: topologyDriver,
	})
	ts := httptest.NewServer(s)
	cleanup := ts.Close

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
		Client:         client,
		Addressbook:    addressbook,
		TopologyDriver: topologyDriver,
		Cleanup:        cleanup,
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
