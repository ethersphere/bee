// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	pingpongmock "github.com/ethersphere/bee/v2/pkg/pingpong/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestPingpong(t *testing.T) {
	t.Parallel()

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

	ts, _, _, _ := newTestServer(t, testServerOptions{
		Pingpong: pingpongService,
	})

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, ts, http.MethodPost, "/pingpong/"+peerID.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.PingpongResponse{
				RTT: rtt.String(),
			}),
		)
	})

	t.Run("peer not found", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, ts, http.MethodPost, "/pingpong/"+unknownPeerID.String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: "peer not found",
			}),
		)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, ts, http.MethodPost, "/pingpong/"+errorPeerID.String(), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: "pingpong: ping failed",
			}),
		)
	})
}

func Test_pingpongHandler_invalidInputs(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodPost, "/pingpong/"+tc.address, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}
