// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/p2p"
	pingpongmock "github.com/ethersphere/bee/pkg/pingpong/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestPingpong(t *testing.T) {
	rtt := time.Minute
	peerID := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	unknownPeerID := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59e")
	errorPeerID := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a")
	testErr := errors.New("test error")

	pingpongService := pingpongmock.New(func(ctx context.Context, address swarm.Address, msgs ...string) (time.Duration, error) {
		if address.Equal(errorPeerID) {
			return 0, testErr
		}
		if !address.Equal(peerID) {
			return 0, p2p.ErrPeerNotFound
		}
		return rtt, nil
	})

	ts := newTestServer(t, testServerOptions{
		Pingpong: pingpongService,
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/pingpong/"+peerID.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.PingpongResponse{
				RTT: rtt.String(),
			}),
		)
	})

	t.Run("peer not found", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/pingpong/"+unknownPeerID.String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: "peer not found",
			}),
		)
	})

	t.Run("invalid peer address", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/pingpong/invalid-address", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid peer address",
			}),
		)
	})

	t.Run("error", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/pingpong/"+errorPeerID.String(), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: http.StatusText(http.StatusInternalServerError), // do not leak internal error
			}),
		)
	})

	t.Run("get method not allowed", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/pingpong/"+peerID.String(), http.StatusMethodNotAllowed,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusMethodNotAllowed,
				Message: http.StatusText(http.StatusMethodNotAllowed),
			}),
		)
	})
}
