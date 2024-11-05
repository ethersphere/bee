// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"errors"
	"math/big"
	"net/http"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestSettlements(t *testing.T) {
	t.Parallel()

	settlementsSentFunc := func() (ret map[string]*big.Int, err error) {
		ret = make(map[string]*big.Int)
		ret["DEAD"] = big.NewInt(10000)
		ret["BEEF"] = big.NewInt(20000)
		ret["FFFF"] = big.NewInt(50000)
		return ret, err
	}

	settlementsRecvFunc := func() (ret map[string]*big.Int, err error) {
		ret = make(map[string]*big.Int)
		ret["BEEF"] = big.NewInt(10000)
		ret["EEEE"] = big.NewInt(5000)
		return ret, err
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []mock.Option{mock.WithSettlementsSentFunc(settlementsSentFunc), mock.WithSettlementsRecvFunc(settlementsRecvFunc)},
	})

	expected := &api.SettlementsResponse{
		TotalSettlementReceived: bigint.Wrap(big.NewInt(15000)),
		TotalSettlementSent:     bigint.Wrap(big.NewInt(80000)),
		Settlements: []api.SettlementResponse{
			{
				Peer:               "DEAD",
				SettlementReceived: bigint.Wrap(big.NewInt(0)),
				SettlementSent:     bigint.Wrap(big.NewInt(10000)),
			},
			{
				Peer:               "BEEF",
				SettlementReceived: bigint.Wrap(big.NewInt(10000)),
				SettlementSent:     bigint.Wrap(big.NewInt(20000)),
			},
			{
				Peer:               "FFFF",
				SettlementReceived: bigint.Wrap(big.NewInt(0)),
				SettlementSent:     bigint.Wrap(big.NewInt(50000)),
			},
			{
				Peer:               "EEEE",
				SettlementReceived: bigint.Wrap(big.NewInt(5000)),
				SettlementSent:     bigint.Wrap(big.NewInt(0)),
			},
		},
	}

	// We expect a list of items unordered by peer:
	var got *api.SettlementsResponse
	jsonhttptest.Request(t, testServer, http.MethodGet, "/settlements", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !equalSettlements(got, expected) {
		t.Errorf("got settlements: %+v, expected: %+v", got, expected)
	}
}

func TestSettlementsError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("New errors")
	settlementsSentFunc := func() (map[string]*big.Int, error) {
		return nil, wantErr
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []mock.Option{mock.WithSettlementsSentFunc(settlementsSentFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/settlements", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrCantSettlements,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestSettlementsPeers(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	settlementSentFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(1000000000000000000), nil
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []mock.Option{mock.WithSettlementSentFunc(settlementSentFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/settlements/"+peer, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(api.SettlementResponse{
			Peer:               peer,
			SettlementSent:     bigint.Wrap(big.NewInt(1000000000000000000)),
			SettlementReceived: bigint.Wrap(big.NewInt(0)),
		}),
	)
}

func TestSettlementsPeersNoSettlements(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	noErrFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(1000000000000000000), nil
	}
	errFunc := func(swarm.Address) (*big.Int, error) {
		return nil, settlement.ErrPeerNoSettlements
	}

	t.Run("no sent", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			SwapOpts: []mock.Option{
				mock.WithSettlementSentFunc(errFunc),
				mock.WithSettlementRecvFunc(noErrFunc),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/settlements/"+peer, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.SettlementResponse{
				Peer:               peer,
				SettlementSent:     bigint.Wrap(big.NewInt(0)),
				SettlementReceived: bigint.Wrap(big.NewInt(1000000000000000000)),
			}),
		)
	})

	t.Run("no received", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			SwapOpts: []mock.Option{
				mock.WithSettlementSentFunc(noErrFunc),
				mock.WithSettlementRecvFunc(errFunc),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/settlements/"+peer, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.SettlementResponse{
				Peer:               peer,
				SettlementSent:     bigint.Wrap(big.NewInt(1000000000000000000)),
				SettlementReceived: bigint.Wrap(big.NewInt(0)),
			}),
		)
	})
}

func Test_peerSettlementsHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name string
		peer string
		want jsonhttp.StatusResponse
	}{{
		name: "peer - odd hex string",
		peer: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "peer",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name: "peer - invalid hex character",
		peer: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "peer",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/settlements/"+tc.peer, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func TestSettlementsPeersError(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	settlementSentFunc := func(swarm.Address) (*big.Int, error) {
		return nil, wantErr
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []mock.Option{mock.WithSettlementSentFunc(settlementSentFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/settlements/"+peer, http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrCantSettlementsPeer,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func equalSettlements(a, b *api.SettlementsResponse) bool {
	var state bool

	for akeys := range a.Settlements {
		state = false
		for bkeys := range b.Settlements {
			if reflect.DeepEqual(a.Settlements[akeys], b.Settlements[bkeys]) {
				state = true
				break
			}
		}
		if !state {
			return false
		}
	}

	for bkeys := range b.Settlements {
		state = false
		for akeys := range a.Settlements {
			if reflect.DeepEqual(a.Settlements[akeys], b.Settlements[bkeys]) {
				state = true
				break
			}
		}
		if !state {
			return false
		}
	}

	if a.TotalSettlementReceived.Cmp(b.TotalSettlementReceived.Int) != 0 {
		return false
	}

	if a.TotalSettlementSent.Cmp(b.TotalSettlementSent.Int) != 0 {
		return false
	}

	return true
}
