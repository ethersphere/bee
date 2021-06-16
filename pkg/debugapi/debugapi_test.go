// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"crypto/ecdsa"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee"
	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/resolver"
	chequebookmock "github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock"
	swapmock "github.com/ethersphere/bee/pkg/settlement/swap/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	topologymock "github.com/ethersphere/bee/pkg/topology/mock"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
	"github.com/multiformats/go-multiaddr"
	"resenje.org/web"
)

type testServerOptions struct {
	Overlay            swarm.Address
	PublicKey          ecdsa.PublicKey
	PSSPublicKey       ecdsa.PublicKey
	EthereumAddress    common.Address
	CORSAllowedOrigins []string
	P2P                *p2pmock.Service
	Pingpong           pingpong.Interface
	Storer             storage.Storer
	Resolver           resolver.Interface
	TopologyOpts       []topologymock.Option
	Tags               *tags.Tags
	AccountingOpts     []accountingmock.Option
	SettlementOpts     []swapmock.Option
	ChequebookOpts     []chequebookmock.Option
	SwapOpts           []swapmock.Option
	BatchStore         postage.Storer
	TransactionOpts    []transactionmock.Option
}

type testServer struct {
	Client  *http.Client
	P2PMock *p2pmock.Service
}

func newTestServer(t *testing.T, o testServerOptions) *testServer {
	topologyDriver := topologymock.NewTopologyDriver(o.TopologyOpts...)
	acc := accountingmock.NewAccounting(o.AccountingOpts...)
	settlement := swapmock.New(o.SettlementOpts...)
	chequebook := chequebookmock.NewChequebook(o.ChequebookOpts...)
	swapserv := swapmock.New(o.SwapOpts...)
	transaction := transactionmock.New(o.TransactionOpts...)
	ln := lightnode.NewContainer(o.Overlay)
	s := debugapi.New(o.Overlay, o.PublicKey, o.PSSPublicKey, o.EthereumAddress, logging.New(ioutil.Discard, 0), nil, o.CORSAllowedOrigins)
	s.Configure(o.P2P, o.Pingpong, topologyDriver, ln, o.Storer, o.Tags, acc, settlement, true, swapserv, chequebook, o.BatchStore, transaction)
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

func mustMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	t.Helper()

	a, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestServer_Configure validates that http routes are correct when server is
// constructed with only basic routes and after it is configured with
// dependencies.
func TestServer_Configure(t *testing.T) {
	privateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	pssPrivateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	addresses := []multiaddr.Multiaddr{
		mustMultiaddr(t, "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/192.168.0.101/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/127.0.0.1/udp/7071/quic/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
	}

	ethereumAddress := common.HexToAddress("abcd")

	o := testServerOptions{
		PublicKey:       privateKey.PublicKey,
		PSSPublicKey:    pssPrivateKey.PublicKey,
		Overlay:         overlay,
		EthereumAddress: ethereumAddress,
		P2P: p2pmock.New(p2pmock.WithAddressesFunc(func() ([]multiaddr.Multiaddr, error) {
			return addresses, nil
		})),
	}
	topologyDriver := topologymock.NewTopologyDriver(o.TopologyOpts...)
	acc := accountingmock.NewAccounting(o.AccountingOpts...)
	settlement := swapmock.New(o.SettlementOpts...)
	chequebook := chequebookmock.NewChequebook(o.ChequebookOpts...)
	swapserv := swapmock.New(o.SwapOpts...)
	ln := lightnode.NewContainer(o.Overlay)
	transaction := transactionmock.New(o.TransactionOpts...)
	s := debugapi.New(o.Overlay, o.PublicKey, o.PSSPublicKey, o.EthereumAddress, logging.New(ioutil.Discard, 0), nil, nil)
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

	testBasicRouter(t, client)
	jsonhttptest.Request(t, client, http.MethodGet, "/readiness", http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusNotFound),
			Code:    http.StatusNotFound,
		}),
	)
	jsonhttptest.Request(t, client, http.MethodGet, "/addresses", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.AddressesResponse{
			Overlay:      o.Overlay,
			Underlay:     make([]multiaddr.Multiaddr, 0),
			Ethereum:     o.EthereumAddress,
			PublicKey:    hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&o.PublicKey)),
			PSSPublicKey: hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&o.PSSPublicKey)),
		}),
	)

	s.Configure(o.P2P, o.Pingpong, topologyDriver, ln, o.Storer, o.Tags, acc, settlement, true, swapserv, chequebook, nil, transaction)

	testBasicRouter(t, client)
	jsonhttptest.Request(t, client, http.MethodGet, "/readiness", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.StatusResponse{
			Status:  "ok",
			Version: bee.Version,
		}),
	)
	jsonhttptest.Request(t, client, http.MethodGet, "/addresses", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.AddressesResponse{
			Overlay:      o.Overlay,
			Underlay:     addresses,
			Ethereum:     o.EthereumAddress,
			PublicKey:    hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&o.PublicKey)),
			PSSPublicKey: hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&o.PSSPublicKey)),
		}),
	)
}

func testBasicRouter(t *testing.T, client *http.Client) {
	t.Helper()

	jsonhttptest.Request(t, client, http.MethodGet, "/health", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.StatusResponse{
			Status:  "ok",
			Version: bee.Version,
		}),
	)

	for _, path := range []string{
		"/metrics",
		"/debug/pprof",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile?seconds=1", // profile for only 1 second to check only the status code
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/vars",
	} {
		jsonhttptest.Request(t, client, http.MethodGet, path, http.StatusOK)
	}
}
