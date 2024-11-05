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
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

	t.Run("error - add peer", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
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

		jsonhttptest.Request(t, testServer, http.MethodDelete, "/peers/"+unknownAddress.String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: "peer not found",
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
}

func TestBlocklistedPeers(t *testing.T) {
	t.Parallel()

	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithBlocklistedPeersFunc(func() ([]p2p.BlockListedPeer, error) {
			return []p2p.BlockListedPeer{{Peer: p2p.Peer{Address: overlay}}}, nil
		})),
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/blocklist", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(api.BlockedListedPeersResponse{
			Peers: []api.BlockListedPeer{{Peer: api.Peer{Address: overlay}, Duration: 0}},
		}),
	)
}

func TestBlocklistedPeersErr(t *testing.T) {
	t.Parallel()

	overlay := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithBlocklistedPeersFunc(func() ([]p2p.BlockListedPeer, error) {
			return []p2p.BlockListedPeer{{Peer: p2p.Peer{Address: overlay}}}, errors.New("some error")
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

func Test_peerConnectHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name         string
		multiAddress string
		want         jsonhttp.StatusResponse
	}{{
		name:         "multi-address - invalid value",
		multiAddress: "ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "multi-address",
					Error: "failed to parse multiaddr \"/ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a\": unknown protocol ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a",
				},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodPost, "/connect/"+tc.multiAddress, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func Test_peerDisconnectHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		address string
		want    jsonhttp.StatusResponse
	}{{
		name:    "address - odd hex string",
		address: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "address",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "address - invalid hex character",
		address: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "address",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodDelete, "/peers/"+tc.address, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}
