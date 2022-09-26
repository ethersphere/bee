// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

func TestConnect(t *testing.T) {
	t.Parallel()

	underlay := "/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS"
	errorUnderlay := "/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkw88cjH2orYrB6fDui4eUNdmgkwnDM8W681UbfsPgM9QY"
	testErr := errors.New("test error")

	privateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	block := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(privateKey.PublicKey, 0, block)
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

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		DebugAPI: true,
		P2P: mock.New(mock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (*bzz.Address, error) {
			if addr.String() == errorUnderlay {
				return nil, testErr
			}
			return bzzAddress, nil
		})),
	})

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		jsonhttptest.Request(t, testServer, http.MethodPost, "/connect"+underlay, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PeerConnectResponse{
				Address: overlay.String(),
			}),
		)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		jsonhttptest.Request(t, testServer, http.MethodPost, "/connect"+errorUnderlay, http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: testErr.Error(),
			}),
		)
	})

	t.Run("get method not allowed", func(t *testing.T) {
		t.Parallel()
		jsonhttptest.Request(t, testServer, http.MethodGet, "/connect"+underlay, http.StatusMethodNotAllowed,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusMethodNotAllowed,
				Message: http.StatusText(http.StatusMethodNotAllowed),
			}),
		)
	})

	t.Run("error - add peer", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI: true,
			P2P: mock.New(mock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (*bzz.Address, error) {
				if addr.String() == errorUnderlay {
					return nil, testErr
				}
				return bzzAddress, nil
			})),
		})

		jsonhttptest.Request(t, testServer, http.MethodPost, "/connect"+errorUnderlay, http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: testErr.Error(),
			}),
		)
	})
}

func TestDisconnect(t *testing.T) {
	t.Parallel()

	address := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	unknownAddress := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59e")
	errorAddress := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a")
	testErr := errors.New("test error")

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		DebugAPI: true,
		P2P: mock.New(mock.WithDisconnectFunc(func(addr swarm.Address, reason string) error {
			if reason != "user requested disconnect" {
				return testErr
			}

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
		t.Parallel()

		jsonhttptest.Request(t, testServer, http.MethodDelete, "/peers/"+address.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusOK,
				Message: http.StatusText(http.StatusOK),
			}),
		)
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, testServer, http.MethodDelete, "/peers/"+unknownAddress.String(), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "peer not found",
			}),
		)
	})

	t.Run("invalid peer address", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, testServer, http.MethodDelete, "/peers/invalid-address", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid peer address",
			}),
		)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, testServer, http.MethodDelete, "/peers/"+errorAddress.String(), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: testErr.Error(),
			}),
		)
	})
}

func TestPeer(t *testing.T) {
	t.Parallel()

	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		DebugAPI: true,
		P2P: mock.New(mock.WithPeersFunc(func() []p2p.Peer {
			return []p2p.Peer{{Address: overlay}}
		})),
	})

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, testServer, http.MethodGet, "/peers", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PeersResponse{
				Peers: []api.Peer{{Address: overlay}},
			}),
		)
	})

	t.Run("get method not allowed", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, testServer, http.MethodPost, "/peers", http.StatusMethodNotAllowed,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusMethodNotAllowed,
				Message: http.StatusText(http.StatusMethodNotAllowed),
			}),
		)
	})
}

func TestBlocklistedPeers(t *testing.T) {
	t.Parallel()

	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		DebugAPI: true,
		P2P: mock.New(mock.WithBlocklistedPeersFunc(func() ([]p2p.Peer, error) {
			return []p2p.Peer{{Address: overlay}}, nil
		})),
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/blocklist", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(api.PeersResponse{
			Peers: []api.Peer{{Address: overlay}},
		}),
	)
}

func TestBlocklistedPeersErr(t *testing.T) {
	t.Parallel()

	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		DebugAPI: true,
		P2P: mock.New(mock.WithBlocklistedPeersFunc(func() ([]p2p.Peer, error) {
			return []p2p.Peer{{Address: overlay}}, errors.New("some error")
		})),
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/blocklist", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(
			jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: "get blocklisted peers failed",
			}),
	)
}
