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

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/logging"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	topologymock "github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/multiformats/go-multiaddr"
	"resenje.org/web"
)

type testServerOptions struct {
	Overlay        swarm.Address
	P2P            *p2pmock.Service
	Pingpong       pingpong.Interface
	Storer         storage.Storer
	TopologyOpts   []topologymock.Option
	Tags           *tags.Tags
	AccountingOpts []accountingmock.Option
}

type testServer struct {
	Client  *http.Client
	P2PMock *p2pmock.Service
}

func newTestServer(t *testing.T, o testServerOptions) *testServer {
	topologyDriver := topologymock.NewTopologyDriver(o.TopologyOpts...)
	acc := accountingmock.NewAccounting(o.AccountingOpts...)

	s := debugapi.New(debugapi.Options{
		Overlay:        o.Overlay,
		P2P:            o.P2P,
		Pingpong:       o.Pingpong,
		Tags:           o.Tags,
		Logger:         logging.New(ioutil.Discard, 0),
		Storer:         o.Storer,
		TopologyDriver: topologyDriver,
		Accounting:     acc,
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
		Client:  client,
		P2PMock: o.P2P,
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
