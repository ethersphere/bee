// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/multiformats/go-multiaddr"
)

func TestAddresses(t *testing.T) {
	addresses := []multiaddr.Multiaddr{
		mustMultiaddr(t, "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/192.168.0.101/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/127.0.0.1/udp/7071/quic/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
	}

	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithAddressesFunc(func() ([]multiaddr.Multiaddr, error) {
			return addresses, nil
		})),
	})
	defer testServer.Cleanup()

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/addresses", nil, http.StatusOK, debugapi.AddressesResponse{
			Addresses: addresses,
		})
	})

	t.Run("post method not allowed", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodPost, "/addresses", nil, http.StatusMethodNotAllowed, jsonhttp.StatusResponse{
			Code:    http.StatusMethodNotAllowed,
			Message: http.StatusText(http.StatusMethodNotAllowed),
		})
	})
}

func TestAddresses_error(t *testing.T) {
	testErr := errors.New("test error")

	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithAddressesFunc(func() ([]multiaddr.Multiaddr, error) {
			return nil, testErr
		})),
	})
	defer testServer.Cleanup()

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/addresses", nil, http.StatusInternalServerError, jsonhttp.StatusResponse{
		Code:    http.StatusInternalServerError,
		Message: testErr.Error(),
	})
}
