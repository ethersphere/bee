// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

func TestConnect(t *testing.T) {
	underlay := "/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS"
	errorUnderlay := "/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkw88cjH2orYrB6fDui4eUNdmgkwnDM8W681UbfsPgM9QY"
	testErr := errors.New("test error")

	privateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err := crypto.NewOverlayAddress(privateKey.PublicKey, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	underlama, err := ma.NewMultiaddr(underlay)
	if err != nil {
		t.Fatal(err)
	}

	bzzAddress, err := bzz.NewAddress(crypto.NewDefaultSigner(privateKey), underlama, overlay, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (*bzz.Address, error) {
			if addr.String() == errorUnderlay {
				return nil, testErr
			}
			return bzzAddress, nil
		})),
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/connect"+underlay, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.PeerConnectResponse{
				Address: overlay.String(),
			}),
		)
	})

	t.Run("error", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/connect"+errorUnderlay, http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: testErr.Error(),
			}),
		)
	})

	t.Run("get method not allowed", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/connect"+underlay, http.StatusMethodNotAllowed,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusMethodNotAllowed,
				Message: http.StatusText(http.StatusMethodNotAllowed),
			}),
		)
	})

	t.Run("error - add peer", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			P2P: mock.New(mock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (*bzz.Address, error) {
				if addr.String() == errorUnderlay {
					return nil, testErr
				}
				return bzzAddress, nil
			})),
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/connect"+errorUnderlay, http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: testErr.Error(),
			}),
		)
	})
}

func TestDisconnect(t *testing.T) {
	address := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	unknownAdddress := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59e")
	errorAddress := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a")
	testErr := errors.New("test error")

	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithDisconnectFunc(func(addr swarm.Address) error {
			if addr.Equal(address) {
				return nil
			}

			if addr.Equal(errorAddress) {
				return testErr
			}

			return p2p.ErrPeerNotFound
		})),
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodDelete, "/peers/"+address.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusOK,
				Message: http.StatusText(http.StatusOK),
			}),
		)
	})

	t.Run("unknown", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodDelete, "/peers/"+unknownAdddress.String(), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "peer not found",
			}),
		)
	})

	t.Run("invalid peer address", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodDelete, "/peers/invalid-address", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid peer address",
			}),
		)
	})

	t.Run("error", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodDelete, "/peers/"+errorAddress.String(), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: testErr.Error(),
			}),
		)
	})
}

func TestPeer(t *testing.T) {
	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithPeersFunc(func() []p2p.Peer {
			return []p2p.Peer{{Address: overlay}}
		})),
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/peers", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.PeersResponse{
				Peers: []p2p.Peer{{Address: overlay}},
			}),
		)
	})

	t.Run("get method not allowed", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/peers", http.StatusMethodNotAllowed,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusMethodNotAllowed,
				Message: http.StatusText(http.StatusMethodNotAllowed),
			}),
		)
	})
}

func TestBlocklistedPeers(t *testing.T) {
	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithBlocklistedPeersFunc(func() ([]p2p.Peer, error) {
			return []p2p.Peer{{Address: overlay}}, nil
		})),
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/blocklist", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.PeersResponse{
			Peers: []p2p.Peer{{Address: overlay}},
		}),
	)
}

func TestBlocklistedPeersErr(t *testing.T) {
	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithBlocklistedPeersFunc(func() ([]p2p.Peer, error) {
			return []p2p.Peer{{Address: overlay}}, errors.New("some error")
		})),
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/blocklist", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(
			jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: http.StatusText(http.StatusInternalServerError),
			}),
	)
}
