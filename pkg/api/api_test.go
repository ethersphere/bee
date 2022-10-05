// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/auth"
	mockauth "github.com/ethersphere/bee/pkg/auth/mock"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/log"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/resolver"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	chequebookmock "github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock"
	erc20mock "github.com/ethersphere/bee/pkg/settlement/swap/erc20/mock"
	swapmock "github.com/ethersphere/bee/pkg/settlement/swap/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/steward"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	topologymock "github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
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

// nolint:gochecknoinits
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
	WsPingPeriod       time.Duration
	Logger             log.Logger
	PreventRedirect    bool
	Feeds              feeds.Factory
	CORSAllowedOrigins []string
	PostageContract    postagecontract.Interface
	StakingContract    staking.Contract
	Post               postage.Service
	Steward            steward.Interface
	WsHeaders          http.Header
	Authenticator      auth.Authenticator
	DebugAPI           bool
	Restricted         bool
	DirectUpload       bool
	Probe              *api.Probe

	Overlay         swarm.Address
	PublicKey       ecdsa.PublicKey
	PSSPublicKey    ecdsa.PublicKey
	EthereumAddress common.Address
	BlockTime       *big.Int
	P2P             *p2pmock.Service
	Pingpong        pingpong.Interface
	TopologyOpts    []topologymock.Option
	AccountingOpts  []accountingmock.Option
	ChequebookOpts  []chequebookmock.Option
	SwapOpts        []swapmock.Option
	TransactionOpts []transactionmock.Option
	Traverser       traversal.Traverser

	BatchStore postage.Storer
	SyncStatus func() (bool, error)

	BackendOpts []backendmock.Option
	Erc20Opts   []erc20mock.Option
	ChainID     int64
}

func newTestServer(t *testing.T, o testServerOptions) (*http.Client, *websocket.Conn, string, *chanStorer) {
	t.Helper()
	pk, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(pk)

	if o.Logger == nil {
		o.Logger = log.Noop
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
	if o.BatchStore == nil {
		o.BatchStore = mockbatchstore.New(mockbatchstore.WithAcceptAllExistsFunc()) // default is with accept-all Exists() func
	}
	if o.SyncStatus == nil {
		o.SyncStatus = func() (bool, error) { return true, nil }
	}
	if o.Authenticator == nil {
		o.Authenticator = &mockauth.Auth{
			EnforceFunc: func(_, _, _ string) (bool, error) {
				return true, nil
			},
		}
	}
	var chanStore *chanStorer

	topologyDriver := topologymock.NewTopologyDriver(o.TopologyOpts...)
	acc := accountingmock.NewAccounting(o.AccountingOpts...)
	settlement := swapmock.New(o.SwapOpts...)
	chequebook := chequebookmock.NewChequebook(o.ChequebookOpts...)
	ln := lightnode.NewContainer(o.Overlay)
	transaction := transactionmock.New(o.TransactionOpts...)

	storeRecipient := statestore.NewStateStore()
	recipient := pseudosettle.New(nil, o.Logger, storeRecipient, nil, big.NewInt(10000), big.NewInt(10000), o.P2P)

	erc20 := erc20mock.New(o.Erc20Opts...)
	backend := backendmock.New(o.BackendOpts...)

	var extraOpts = api.ExtraOptions{
		TopologyDriver:   topologyDriver,
		Accounting:       acc,
		Pseudosettle:     recipient,
		LightNodes:       ln,
		Swap:             settlement,
		Chequebook:       chequebook,
		Pingpong:         o.Pingpong,
		BlockTime:        o.BlockTime,
		Tags:             o.Tags,
		Storer:           o.Storer,
		Resolver:         o.Resolver,
		Pss:              o.Pss,
		TraversalService: o.Traversal,
		Pinning:          o.Pinning,
		FeedFactory:      o.Feeds,
		Post:             o.Post,
		PostageContract:  o.PostageContract,
		Steward:          o.Steward,
		SyncStatus:       o.SyncStatus,
		Staking:          o.StakingContract,
	}

	s := api.New(o.PublicKey, o.PSSPublicKey, o.EthereumAddress, o.Logger, transaction, o.BatchStore, api.FullMode, true, true, backend, o.CORSAllowedOrigins)

	s.SetP2P(o.P2P)
	s.SetSwarmAddress(&o.Overlay)

	s.SetProbe(o.Probe)

	noOpTracer, tracerCloser, _ := tracing.NewTracer(&tracing.Options{
		Enabled: false,
	})

	t.Cleanup(func() { _ = tracerCloser.Close() })

	chC := s.Configure(signer, o.Authenticator, noOpTracer, api.Options{
		CORSAllowedOrigins: o.CORSAllowedOrigins,
		WsPingPeriod:       o.WsPingPeriod,
		Restricted:         o.Restricted,
	}, extraOpts, 1, erc20)

	if o.DebugAPI {
		s.MountTechnicalDebug()
		s.MountDebug(false)
	} else {
		s.MountAPI()
	}

	if o.DirectUpload {
		chanStore = newChanStore(chC)
		t.Cleanup(chanStore.stop)
	}

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

	if !o.DebugAPI {
		httpClient = &http.Client{
			Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				requestURL := r.URL.String()
				if r.URL.Scheme != "http" {
					requestURL = ts.URL + r.URL.String()
				}
				u, err := url.Parse(requestURL)
				if err != nil {
					return nil, err
				}
				r.URL = u

				transport := ts.Client().Transport.(*http.Transport)
				transport = transport.Clone()
				// always dial to the server address, regardless of the url host and port
				transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial(network, ts.Listener.Addr().String())
				}
				return transport.RoundTrip(r)
			}),
		}
	}

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

	return httpClient, conn, ts.Listener.Addr().String(), chanStore
}

func request(t *testing.T, client *http.Client, method, resource string, body io.Reader, responseCode int) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.TODO(), method, resource, body)
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
	t.Parallel()

	const bzzHash = "89c17d0d8018a19057314aa035e61c9d23c47581a61dd3a79a7839692c617e4d"
	log := log.Noop

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
					return swarm.ZeroAddress, api.ErrInvalidNameOrAddress
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

		s := api.New(pk.PublicKey, pk.PublicKey, common.Address{}, log, nil, nil, 1, false, false, nil, []string{"*"})
		s.Configure(signer, nil, nil, api.Options{}, api.ExtraOptions{Resolver: tC.res}, 1, nil)
		s.MountAPI()

		tC := tC
		t.Run(tC.desc, func(t *testing.T) {
			t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	var (
		mockStorer      = mock.NewStorer()
		mockStatestore  = statestore.NewStateStore()
		logger          = log.Noop
		mp              = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)))
		client, _, _, _ = newTestServer(t, testServerOptions{
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
		endpoint := endpoint
		t.Run(endpoint+": empty batch", func(t *testing.T) {
			t.Parallel()

			hexbatch := hex.EncodeToString(batchEmpty)
			expCode := http.StatusBadRequest
			jsonhttptest.Request(t, client, http.MethodPost, "/"+endpoint, expCode,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			)
		})
		t.Run(endpoint+": ok batch", func(t *testing.T) {
			t.Parallel()

			hexbatch := hex.EncodeToString(batchOk)
			expCode := http.StatusCreated
			jsonhttptest.Request(t, client, http.MethodPost, "/"+endpoint, expCode,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			)
		})
		t.Run(endpoint+": bad batch", func(t *testing.T) {
			t.Parallel()

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

// TestOptions check whether endpoint compatible with option method
func TestOptions(t *testing.T) {
	t.Parallel()

	var (
		client, _, _, _ = newTestServer(t, testServerOptions{})
	)
	for _, tc := range []struct {
		endpoint        string
		expectedMethods string // expectedMethods contains HTTP methods like GET, POST, HEAD, PATCH, DELETE, OPTIONS. These are in alphabetical sorted order
	}{
		{
			endpoint:        "tags",
			expectedMethods: "GET, POST",
		},
		{
			endpoint:        "bzz",
			expectedMethods: "POST",
		},
		{
			endpoint:        "chunks",
			expectedMethods: "POST",
		},
		{
			endpoint:        "chunks/123213",
			expectedMethods: "DELETE, GET, HEAD",
		},
		{
			endpoint:        "bytes",
			expectedMethods: "POST",
		},
		{
			endpoint:        "bytes/0121012",
			expectedMethods: "GET, HEAD",
		},
	} {
		tc := tc
		t.Run(tc.endpoint+" options test", func(t *testing.T) {
			t.Parallel()

			resultHeader := jsonhttptest.Request(t, client, http.MethodOptions, "/"+tc.endpoint, http.StatusNoContent)

			allowedMethods := resultHeader.Get("Allow")
			if allowedMethods != tc.expectedMethods {
				t.Fatalf("expects %s and got %s", tc.expectedMethods, allowedMethods)
			}
		})
	}
}

// TestPostageDirectAndDeferred tests that incorrect postage batch ids
// provided to the api correct the appropriate error code.
func TestPostageDirectAndDeferred(t *testing.T) {
	t.Parallel()

	var (
		mockStorer               = mock.NewStorer()
		mockStatestore           = statestore.NewStateStore()
		logger                   = log.Noop
		mp                       = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)))
		client, _, _, chanStorer = newTestServer(t, testServerOptions{
			Storer:       mockStorer,
			Tags:         tags.NewTags(mockStatestore, logger),
			Logger:       logger,
			Post:         mp,
			DirectUpload: true,
		})

		endpoints = []string{
			"bytes", "bzz", "chunks",
		}
	)
	for _, endpoint := range endpoints {
		endpoint := endpoint
		t.Run(endpoint+": deferred", func(t *testing.T) {
			t.Parallel()

			hexbatch := hex.EncodeToString(batchOk)
			chunk := testingc.GenerateTestRandomChunk()
			var responseBytes []byte
			jsonhttptest.Request(t, client, http.MethodPost, "/"+endpoint, http.StatusCreated,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
				jsonhttptest.WithPutResponseBody(&responseBytes),
			)
			var body struct {
				Reference swarm.Address `json:"reference"`
			}
			if err := json.Unmarshal(responseBytes, &body); err != nil {
				t.Fatal("unmarshal response body:", err)
			}
			if found, _ := mockStorer.Has(context.Background(), body.Reference); !found {
				t.Fatal("chunk not found in the store")
			}
			if found, _ := chanStorer.Has(context.Background(), body.Reference); found {
				t.Fatal("chunk was not expected to be present in direct channel")
			}
		})
		t.Run(endpoint+": direct upload", func(t *testing.T) {
			t.Parallel()

			hexbatch := hex.EncodeToString(batchOk)
			chunk := testingc.GenerateTestRandomChunk()
			var responseBytes []byte
			jsonhttptest.Request(t, client, http.MethodPost, "/"+endpoint, http.StatusCreated,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "false"),
				jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
				jsonhttptest.WithPutResponseBody(&responseBytes),
			)

			var body struct {
				Reference swarm.Address `json:"reference"`
			}
			if err := json.Unmarshal(responseBytes, &body); err != nil {
				t.Fatal("unmarshal response body:", err)
			}
			if found, _ := chanStorer.Has(context.Background(), body.Reference); !found {
				t.Fatal("chunk not received through the direct channel")
			}
			if found, _ := mockStorer.Has(context.Background(), body.Reference); found {
				t.Fatal("chunk was not expected to be present in store")
			}
		})
	}
}

type chanStorer struct {
	lock   sync.Mutex
	chunks map[string]struct{}
	quit   chan struct{}
}

func newChanStore(cc <-chan *pusher.Op) *chanStorer {
	c := &chanStorer{
		chunks: make(map[string]struct{}),
		quit:   make(chan struct{}),
	}
	go c.drain(cc)
	return c
}

func (c *chanStorer) drain(cc <-chan *pusher.Op) {
	for {
		select {
		case op := <-cc:
			c.lock.Lock()
			c.chunks[op.Chunk.Address().ByteString()] = struct{}{}
			c.lock.Unlock()
			op.Err <- nil
		case <-c.quit:
			return
		}
	}
}
func (c *chanStorer) stop() {
	close(c.quit)
}

func (c *chanStorer) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) GetMulti(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) (ch []swarm.Chunk, err error) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	c.lock.Lock()
	_, ok := c.chunks[addr.ByteString()]
	c.lock.Unlock()

	return ok, nil
}

func (c *chanStorer) HasMulti(ctx context.Context, addrs ...swarm.Address) (yes []bool, err error) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) LastPullSubscriptionBinID(bin uint8) (id uint64, err error) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) SubscribePull(ctx context.Context, bin uint8, since uint64, until uint64) (<-chan storage.Descriptor, <-chan struct{}, func()) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) SubscribePush(ctx context.Context, skipf func([]byte) bool) (<-chan swarm.Chunk, func(), func()) {
	panic("not implemented") // TODO: Implement
}

func (c *chanStorer) Close() error {
	panic("not implemented") // TODO: Implement
}
