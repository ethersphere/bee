// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/postage"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/resolver"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/steward"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/gorilla/websocket"
	"resenje.org/web"
)

var (
	batchInvalid = []byte{0}
	batchOk      = make([]byte, 32)
	batchOkStr   string
	batchEmpty   = []byte{}
)

func init() {
	_, _ = rand.Read(batchOk)

	batchOkStr = hex.EncodeToString(batchOk)
}

type testServerOptions struct {
	Storer             storage.Storer
	Resolver           resolver.Interface
	Pss                pss.Interface
	Traversal          traversal.Traverser
	Pinning            pinning.Interface
	WsPath             string
	Tags               *tags.Tags
	GatewayMode        bool
	WsPingPeriod       time.Duration
	Logger             logging.Logger
	PreventRedirect    bool
	Feeds              feeds.Factory
	CORSAllowedOrigins []string
	PostageContract    postagecontract.Interface
	Post               postage.Service
	Steward            steward.Interface
	WsHeaders          http.Header
}

func newTestServer(t *testing.T, o testServerOptions) (*http.Client, *websocket.Conn, string) {
	t.Helper()
	pk, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(pk)

	if o.Logger == nil {
		o.Logger = logging.New(io.Discard, 0)
	}
	if o.Resolver == nil {
		o.Resolver = resolverMock.NewResolver()
	}
	if o.WsPingPeriod == 0 {
		o.WsPingPeriod = 60 * time.Second
	}
	if o.Post == nil {
		o.Post = mockpost.New()
	}
	s := api.New(o.Tags, o.Storer, o.Resolver, o.Pss, o.Traversal, o.Pinning, o.Feeds, o.Post, o.PostageContract, o.Steward, signer, nil, o.Logger, nil, api.Options{
		CORSAllowedOrigins: o.CORSAllowedOrigins,
		GatewayMode:        o.GatewayMode,
		WsPingPeriod:       o.WsPingPeriod,
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
		conn, _, err = websocket.DefaultDialer.Dial(u.String(), o.WsHeaders)
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

func request(t *testing.T, client *http.Client, method, resource string, body io.Reader, responseCode int) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, resource, body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != responseCode {
		t.Fatalf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}
	return resp
}

func pipelineFactory(s storage.Putter, mode storage.ModePut, encrypt bool) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(context.Background(), s, mode, encrypt)
	}
}

func TestParseName(t *testing.T) {
	const bzzHash = "89c17d0d8018a19057314aa035e61c9d23c47581a61dd3a79a7839692c617e4d"
	log := logging.New(io.Discard, 0)

	testCases := []struct {
		desc       string
		name       string
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
		if tC.res == nil && !tC.noResolver {
			tC.res = resolverMock.NewResolver(
				resolverMock.WithResolveFunc(func(string) (swarm.Address, error) {
					return tC.wantAdr, nil
				}))
		}

		pk, _ := crypto.GenerateSecp256k1Key()
		signer := crypto.NewDefaultSigner(pk)
		mockPostage := mockpost.New()

		s := api.New(nil, nil, tC.res, nil, nil, nil, nil, mockPostage, nil, nil, signer, nil, log, nil, api.Options{}).(*api.Server)

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

// TestCalculateNumberOfChunks is a unit test for
// the chunk-number-according-to-content-length calculation.
func TestCalculateNumberOfChunks(t *testing.T) {
	for _, tc := range []struct{ len, chunks int64 }{
		{len: 1000, chunks: 1},
		{len: 5000, chunks: 3},
		{len: 10000, chunks: 4},
		{len: 100000, chunks: 26},
		{len: 1000000, chunks: 248},
		{len: 325839339210, chunks: 79550620 + 621490 + 4856 + 38 + 1},
	} {
		res := api.CalculateNumberOfChunks(tc.len, false)
		if res != tc.chunks {
			t.Fatalf("expected result for %d bytes to be %d got %d", tc.len, tc.chunks, res)
		}
	}
}

// TestCalculateNumberOfChunksEncrypted is a unit test for
// the chunk-number-according-to-content-length calculation with encryption
// (branching factor=64)
func TestCalculateNumberOfChunksEncrypted(t *testing.T) {
	for _, tc := range []struct{ len, chunks int64 }{
		{len: 1000, chunks: 1},
		{len: 5000, chunks: 3},
		{len: 10000, chunks: 4},
		{len: 100000, chunks: 26},
		{len: 1000000, chunks: 245 + 4 + 1},
		{len: 325839339210, chunks: 79550620 + 1242979 + 19422 + 304 + 5 + 1},
	} {
		res := api.CalculateNumberOfChunks(tc.len, true)
		if res != tc.chunks {
			t.Fatalf("expected result for %d bytes to be %d got %d", tc.len, tc.chunks, res)
		}
	}
}

// TestPostageHeaderError tests that incorrect postage batch ids
// provided to the api correct the appropriate error code.
func TestPostageHeaderError(t *testing.T) {
	var (
		mockStorer     = mock.NewStorer()
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(io.Discard, 5)
		mp             = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)))
		client, _, _   = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tags.NewTags(mockStatestore, logger),
			Logger: logger,
			Post:   mp,
		})

		endpoints = []string{
			"bytes", "bzz", "chunks",
		}
	)
	content := []byte{7: 0} // 8 zeros
	for _, endpoint := range endpoints {
		t.Run(endpoint+": empty batch", func(t *testing.T) {
			hexbatch := hex.EncodeToString(batchEmpty)
			expCode := http.StatusBadRequest
			jsonhttptest.Request(t, client, http.MethodPost, "/"+endpoint, expCode,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			)
		})
		t.Run(endpoint+": ok batch", func(t *testing.T) {
			hexbatch := hex.EncodeToString(batchOk)
			expCode := http.StatusCreated
			jsonhttptest.Request(t, client, http.MethodPost, "/"+endpoint, expCode,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			)
		})
		t.Run(endpoint+": bad batch", func(t *testing.T) {
			hexbatch := hex.EncodeToString(batchInvalid)
			expCode := http.StatusBadRequest
			jsonhttptest.Request(t, client, http.MethodPost, "/"+endpoint, expCode,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			)
		})
	}
}
