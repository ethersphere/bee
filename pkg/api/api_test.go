// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/resolver"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/gorilla/websocket"
	"resenje.org/web"
)

type testServerOptions struct {
	Storer          storage.Storer
	Resolver        resolver.Interface
	Pss             pss.Interface
	Traversal       traversal.Service
	WsPath          string
	Tags            *tags.Tags
	GatewayMode     bool
	WsPingPeriod    time.Duration
	Logger          logging.Logger
	PreventRedirect bool
}

func newTestServer(t *testing.T, o testServerOptions) (*http.Client, *websocket.Conn, string) {
	if o.Logger == nil {
		o.Logger = logging.New(ioutil.Discard, 0)
	}
	if o.Resolver == nil {
		o.Resolver = resolverMock.NewResolver()
	}
	if o.WsPingPeriod == 0 {
		o.WsPingPeriod = 60 * time.Second
	}
	s := api.New(o.Tags, o.Storer, o.Resolver, o.Pss, o.Traversal, o.Logger, nil, api.Options{
		GatewayMode:  o.GatewayMode,
		WsPingPeriod: o.WsPingPeriod,
	})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	var (
		httpClient = &http.Client{
			Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				u, err := url.Parse(ts.URL + r.URL.String())
				if err != nil {
					return nil, err
				}
				r.URL = u
				return ts.Client().Transport.RoundTrip(r)
			}),
		}
		conn *websocket.Conn
		err  error
	)

	if o.WsPath != "" {
		u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: o.WsPath}
		conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("dial: %v. url %v", err, u.String())
		}
	}

	if o.PreventRedirect {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return httpClient, conn, ts.Listener.Addr().String()
}

func TestParseName(t *testing.T) {
	const bzzHash = "89c17d0d8018a19057314aa035e61c9d23c47581a61dd3a79a7839692c617e4d"

	testCases := []struct {
		desc       string
		name       string
		log        logging.Logger
		res        resolver.Interface
		noResolver bool
		wantAdr    swarm.Address
		wantErr    error
	}{
		{
			desc:    "empty name",
			name:    "",
			wantErr: api.ErrInvalidNameOrAddress,
		},
		{
			desc:    "bzz hash",
			name:    bzzHash,
			wantAdr: swarm.MustParseHexAddress(bzzHash),
		},
		{
			desc:       "no resolver connected with bzz hash",
			name:       bzzHash,
			noResolver: true,
			wantAdr:    swarm.MustParseHexAddress(bzzHash),
		},
		{
			desc:       "no resolver connected with name",
			name:       "itdoesntmatter.eth",
			noResolver: true,
			wantErr:    api.ErrNoResolver,
		},
		{
			desc: "name not resolved",
			name: "not.good",
			res: resolverMock.NewResolver(
				resolverMock.WithResolveFunc(func(string) (swarm.Address, error) {
					return swarm.ZeroAddress, errors.New("failed to resolve")
				}),
			),
			wantErr: api.ErrInvalidNameOrAddress,
		},
		{
			desc:    "name resolved",
			name:    "everything.okay",
			wantAdr: swarm.MustParseHexAddress("89c17d0d8018a19057314aa035e61c9d23c47581a61dd3a79a7839692c617e4d"),
		},
	}
	for _, tC := range testCases {
		if tC.log == nil {
			tC.log = logging.New(ioutil.Discard, 0)
		}
		if tC.res == nil && !tC.noResolver {
			tC.res = resolverMock.NewResolver(
				resolverMock.WithResolveFunc(func(string) (swarm.Address, error) {
					return tC.wantAdr, nil
				}))
		}

		s := api.New(nil, nil, tC.res, nil, nil, tC.log, nil, api.Options{}).(*api.Server)

		t.Run(tC.desc, func(t *testing.T) {
			got, err := s.ResolveNameOrAddress(tC.name)
			if err != nil && !errors.Is(err, tC.wantErr) {
				t.Fatalf("bad error: %v", err)
			}
			if !got.Equal(tC.wantAdr) {
				t.Errorf("got %s, want %s", got, tC.wantAdr)
			}

		})
	}
}
