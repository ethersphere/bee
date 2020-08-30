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

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/resolver"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"resenje.org/web"
)

type testServerOptions struct {
	Pingpong pingpong.Interface
	Storer   storage.Storer
	Resolver resolver.Interface
	Tags     *tags.Tags
	Logger   logging.Logger
}

func newTestServer(t *testing.T, o testServerOptions) *http.Client {
	if o.Logger == nil {
		o.Logger = logging.New(ioutil.Discard, 0)
	}
	if o.Resolver == nil {
		o.Resolver = resolverMock.NewResolver()
	}
	s := api.New(o.Tags, o.Storer, o.Resolver, nil, o.Logger, nil)
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

func TestParseName(t *testing.T) {

	testCases := []struct {
		desc    string
		name    string
		log     logging.Logger
		res     resolver.Interface
		wantAdr swarm.Address
		wantErr error
	}{
		{
			desc:    "empty name",
			name:    "",
			wantErr: api.ErrInvalidChunkAddress,
		},
		{
			desc:    "bzz hash",
			name:    "89c17d0d8018a19057314aa035e61c9d23c47581a61dd3a79a7839692c617e4d",
			wantAdr: swarm.MustParseHexAddress("89c17d0d8018a19057314aa035e61c9d23c47581a61dd3a79a7839692c617e4d"),
		},
		{
			desc: "name not resolved",
			name: "not.good",
			res: resolverMock.NewResolver(
				resolverMock.WithResolveFunc(func(string) (swarm.Address, error) {
					return swarm.ZeroAddress, errors.New("failed to resolve")
				}),
			),
			wantErr: api.ErrInvalidChunkAddress,
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
		if tC.res == nil {
			tC.res = resolverMock.NewResolver(
				resolverMock.WithResolveFunc(func(string) (swarm.Address, error) {
					return tC.wantAdr, nil
				}))
		}

		s := api.New(nil, nil, tC.res, nil, tC.log, nil).(*api.Server)

		t.Run(tC.desc, func(t *testing.T) {
			got, err := s.ResolveNameOrAddress(tC.name)
			if err != nil && !errors.Is(err, api.ErrInvalidChunkAddress) {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tC.wantAdr) {
				t.Errorf("got %s, want %s", got, tC.wantAdr)
			}

		})
	}
}
