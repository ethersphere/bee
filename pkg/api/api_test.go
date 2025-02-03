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
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	mockac "github.com/ethersphere/bee/v2/pkg/accesscontrol/mock"
	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/gsoc"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	p2pmock "github.com/ethersphere/bee/v2/pkg/p2p/mock"
	"github.com/ethersphere/bee/v2/pkg/pingpong"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	contractMock "github.com/ethersphere/bee/v2/pkg/postage/postagecontract/mock"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/resolver"
	resolverMock "github.com/ethersphere/bee/v2/pkg/resolver/mock"
	"github.com/ethersphere/bee/v2/pkg/settlement/pseudosettle"
	chequebookmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook/mock"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	swapmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	statestore "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/steward"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
	mock2 "github.com/ethersphere/bee/v2/pkg/storageincentives/staking/mock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	topologymock "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
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
	Storer             api.Storer
	StateStorer        storage.StateStorer
	Resolver           resolver.Interface
	Pss                pss.Interface
	Gsoc               gsoc.Listener
	WsPath             string
	WsPingPeriod       time.Duration
	Logger             log.Logger
	PreventRedirect    bool
	Feeds              feeds.Factory
	CORSAllowedOrigins []string
	PostageContract    postagecontract.Interface
	StakingContract    staking.Contract
	Post               postage.Service
	AccessControl      accesscontrol.Controller
	Steward            steward.Interface
	WsHeaders          http.Header
	DirectUpload       bool
	Probe              *api.Probe

	Overlay         swarm.Address
	PublicKey       ecdsa.PublicKey
	PSSPublicKey    ecdsa.PublicKey
	EthereumAddress common.Address
	BlockTime       time.Duration
	P2P             *p2pmock.Service
	Pingpong        pingpong.Interface
	TopologyOpts    []topologymock.Option
	AccountingOpts  []accountingmock.Option
	ChequebookOpts  []chequebookmock.Option
	SwapOpts        []swapmock.Option
	TransactionOpts []transactionmock.Option

	BatchStore postage.Storer
	SyncStatus func() (bool, error)

	BackendOpts         []backendmock.Option
	Erc20Opts           []erc20mock.Option
	BeeMode             api.BeeNodeMode
	RedistributionAgent *storageincentives.Agent
	NodeStatus          *status.Service
	PinIntegrity        api.PinIntegrity
	WhitelistedAddr     string
	FullAPIDisabled     bool
	ChequebookDisabled  bool
	SwapDisabled        bool
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
	if o.AccessControl == nil {
		o.AccessControl = mockac.New()
	}
	if o.BatchStore == nil {
		o.BatchStore = mockbatchstore.New(mockbatchstore.WithAcceptAllExistsFunc()) // default is with accept-all Exists() func
	}
	if o.SyncStatus == nil {
		o.SyncStatus = func() (bool, error) { return true, nil }
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

	if o.StateStorer == nil {
		o.StateStorer = storeRecipient
	}
	erc20 := erc20mock.New(o.Erc20Opts...)
	backend := backendmock.New(o.BackendOpts...)

	extraOpts := api.ExtraOptions{
		TopologyDriver:  topologyDriver,
		Accounting:      acc,
		Pseudosettle:    recipient,
		LightNodes:      ln,
		Swap:            settlement,
		Chequebook:      chequebook,
		Pingpong:        o.Pingpong,
		BlockTime:       o.BlockTime,
		Storer:          o.Storer,
		Resolver:        o.Resolver,
		Pss:             o.Pss,
		Gsoc:            o.Gsoc,
		FeedFactory:     o.Feeds,
		Post:            o.Post,
		AccessControl:   o.AccessControl,
		PostageContract: o.PostageContract,
		Steward:         o.Steward,
		SyncStatus:      o.SyncStatus,
		Staking:         o.StakingContract,
		NodeStatus:      o.NodeStatus,
		PinIntegrity:    o.PinIntegrity,
	}

	// By default bee mode is set to full mode.
	if o.BeeMode == api.UnknownMode {
		o.BeeMode = api.FullMode
	}

	s := api.New(o.PublicKey, o.PSSPublicKey, o.EthereumAddress, []string{o.WhitelistedAddr}, o.Logger, transaction, o.BatchStore, o.BeeMode, !o.ChequebookDisabled, !o.SwapDisabled, backend, o.CORSAllowedOrigins, inmemstore.New())
	testutil.CleanupCloser(t, s)

	s.SetP2P(o.P2P)

	if o.RedistributionAgent == nil {
		o.RedistributionAgent, _ = createRedistributionAgentService(t, o.Overlay, o.StateStorer, erc20, transaction, backend, o.BatchStore)
		s.SetRedistributionAgent(o.RedistributionAgent)
	}
	testutil.CleanupCloser(t, o.RedistributionAgent)

	s.SetSwarmAddress(&o.Overlay)
	s.SetProbe(o.Probe)

	noOpTracer, tracerCloser, _ := tracing.NewTracer(&tracing.Options{
		Enabled: false,
	})
	testutil.CleanupCloser(t, tracerCloser)

	s.Configure(signer, noOpTracer, api.Options{
		CORSAllowedOrigins: o.CORSAllowedOrigins,
		WsPingPeriod:       o.WsPingPeriod,
	}, extraOpts, 1, erc20)

	s.Mount()
	if !o.FullAPIDisabled {
		s.EnableFullAPI()
	}

	if o.DirectUpload {
		chanStore = newChanStore(o.Storer.PusherFeed())
		t.Cleanup(chanStore.stop)
	}

	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	var (
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
		conn *websocket.Conn
		err  error
	)

	if o.WsPath != "" {
		u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: o.WsPath}
		conn, _, err = websocket.DefaultDialer.Dial(u.String(), o.WsHeaders)
		if err != nil {
			t.Fatalf("dial: %v. url %v", err, u.String())
		}
		testutil.CleanupCloser(t, conn)
	}

	if o.PreventRedirect {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return httpClient, conn, ts.Listener.Addr().String(), chanStore
}

func pipelineFactory(s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(context.Background(), s, encrypt, rLevel)
	}
}

func TestParseName(t *testing.T) {
	t.Parallel()

	const bzzHash = "89c17d0d8018a19057314aa035e61c9d23c47581a61dd3a79a7839692c617e4d"
	log := log.Noop

	errInvalidNameOrAddress := errors.New("invalid name or bzz address")

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
			wantAdr: swarm.ZeroAddress,
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
					return swarm.ZeroAddress, errInvalidNameOrAddress
				}),
			),
			wantErr: errInvalidNameOrAddress,
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

		s := api.New(pk.PublicKey, pk.PublicKey, common.Address{}, nil, log, nil, nil, 1, false, false, nil, []string{"*"}, inmemstore.New())
		s.Configure(signer, nil, api.Options{}, api.ExtraOptions{Resolver: tC.res}, 1, nil)
		s.Mount()
		s.EnableFullAPI()

		t.Run(tC.desc, func(t *testing.T) {
			t.Parallel()

			got, err := s.ResolveNameOrAddress(tC.name)
			if tC.wantErr != nil && !errors.Is(err, tC.wantErr) {
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
		mockStorer = mockstorer.New()
		endpoints  = []string{
			"bytes", "bzz", "chunks",
		}
	)
	content := []byte{7: 0} // 8 zeros
	for _, endpoint := range endpoints {
		t.Run(endpoint+": empty batch", func(t *testing.T) {
			t.Parallel()

			client, _, _, _ := newTestServer(t, testServerOptions{
				Storer:       mockStorer,
				Post:         newTestPostService(),
				DirectUpload: true,
			})
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
			client, _, _, _ := newTestServer(t, testServerOptions{
				Storer:       mockStorer,
				Post:         newTestPostService(),
				DirectUpload: true,
			})
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
			client, _, _, _ := newTestServer(t, testServerOptions{
				Storer:       mockStorer,
				Post:         newTestPostService(),
				DirectUpload: true,
			})
			hexbatch := hex.EncodeToString(batchInvalid)
			expCode := http.StatusNotFound
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

	client, _, _, _ := newTestServer(t, testServerOptions{})
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
			expectedMethods: "GET, HEAD",
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
		t.Run(tc.endpoint+" options test", func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodOptions, "/"+tc.endpoint, http.StatusNoContent,
				jsonhttptest.WithExpectedResponseHeader("Allow", tc.expectedMethods),
			)
		})
	}
}

// TestPostageDirectAndDeferred tests that incorrect postage batch ids
// provided to the api correct the appropriate error code.
func TestPostageDirectAndDeferred(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"bytes", "bzz", "chunks"} {
		if endpoint != "chunks" {
			t.Run(endpoint+" deferred", func(t *testing.T) {
				t.Parallel()

				mockStorer := mockstorer.New()
				client, _, _, chanStorer := newTestServer(t, testServerOptions{
					Storer:       mockStorer,
					Post:         newTestPostService(),
					DirectUpload: true,
				})
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
				if found, _ := mockStorer.ChunkStore().Has(context.Background(), body.Reference); !found {
					t.Fatal("chunk not found in the store")
				}
				// sleep to allow chanStorer to drain if any to ensure we haven't seen the chunk
				time.Sleep(100 * time.Millisecond)
				if chanStorer.Has(body.Reference) {
					t.Fatal("chunk was not expected to be present in direct channel")
				}
			})
		}

		t.Run(endpoint+" direct upload", func(t *testing.T) {
			t.Parallel()

			mockStorer := mockstorer.New()
			client, _, _, chanStorer := newTestServer(t, testServerOptions{
				Storer:       mockStorer,
				Post:         newTestPostService(),
				DirectUpload: true,
			})
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
			err := spinlock.Wait(time.Second, func() bool { return chanStorer.Has(body.Reference) })
			if err != nil {
				t.Fatal("chunk not found in direct channel")
			}
			if found, _ := mockStorer.ChunkStore().Has(context.Background(), body.Reference); found {
				t.Fatal("chunk was not expected to be present in store")
			}
		})
	}
}

type chanStorer struct {
	lock   sync.Mutex
	chunks map[string]struct{}
	quit   chan struct{}
	subs   []func(chunk swarm.Chunk)
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
			for _, h := range c.subs {
				h(op.Chunk)
			}
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

func (c *chanStorer) Has(addr swarm.Address) bool {
	c.lock.Lock()
	_, ok := c.chunks[addr.ByteString()]
	c.lock.Unlock()

	return ok
}

func (c *chanStorer) Subscribe(f func(chunk swarm.Chunk)) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.subs = append(c.subs, f)
}

func createRedistributionAgentService(
	t *testing.T,
	addr swarm.Address,
	storer storage.StateStorer,
	erc20Service erc20.Service,
	tranService transaction.Service,
	backend storageincentives.ChainBackend,
	chainStateGetter postage.ChainStateGetter,
) (*storageincentives.Agent, error) {
	t.Helper()

	const blocksPerRound uint64 = 12
	const blocksPerPhase uint64 = 4
	postageContract := contractMock.New(contractMock.WithExpiresBatchesFunc(func(context.Context) error {
		return nil
	}),
	)
	stakingContract := mock2.New(mock2.WithIsFrozen(func(context.Context, uint64) (bool, error) {
		return true, nil
	}))
	contract := &mockContract{}

	return storageincentives.New(
		addr,
		common.Address{},
		backend,
		contract,
		postageContract,
		stakingContract,
		mockstorer.NewReserve(),
		func() bool { return true },
		time.Millisecond*10,
		blocksPerRound,
		blocksPerPhase,
		storer,
		chainStateGetter,
		erc20Service,
		tranService,
		&mockHealth{},
		log.Noop,
	)
}

type contractCall int

func (c contractCall) String() string {
	switch c {
	case isWinnerCall:
		return "isWinnerCall"
	case revealCall:
		return "revealCall"
	case commitCall:
		return "commitCall"
	case claimCall:
		return "claimCall"
	}
	return "unknown"
}

const (
	isWinnerCall contractCall = iota
	revealCall
	commitCall
	claimCall
)

type mockContract struct {
	callsList []contractCall
	mtx       sync.Mutex
}

func (m *mockContract) Fee(ctx context.Context, txHash common.Hash) *big.Int {
	return big.NewInt(1000)
}

func (m *mockContract) ReserveSalt(context.Context) ([]byte, error) {
	return nil, nil
}

func (m *mockContract) IsPlaying(context.Context, uint8) (bool, error) {
	return true, nil
}

func (m *mockContract) IsWinner(context.Context) (bool, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, isWinnerCall)
	return false, nil
}

func (m *mockContract) Claim(context.Context, redistribution.ChunkInclusionProofs) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, claimCall)
	return common.Hash{}, nil
}

func (m *mockContract) Commit(context.Context, []byte, uint64) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, commitCall)
	return common.Hash{}, nil
}

func (m *mockContract) Reveal(context.Context, uint8, []byte, []byte) (common.Hash, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.callsList = append(m.callsList, revealCall)
	return common.Hash{}, nil
}

type mockHealth struct{}

func (m *mockHealth) IsHealthy() bool { return true }

func newTestPostService() postage.Service {
	return mockpost.New(
		mockpost.WithIssuer(postage.NewStampIssuer(
			"",
			"",
			batchOk,
			big.NewInt(3),
			11,
			10,
			1000,
			true,
		)),
	)
}
