// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"errors"
	"math/big"
	"net/http"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSettlements(t *testing.T) {
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

	testServer := newTestServer(t, testServerOptions{
		SettlementOpts: []mock.Option{mock.WithSettlementsSentFunc(settlementsSentFunc), mock.WithSettlementsRecvFunc(settlementsRecvFunc)},
	})

	expected := &debugapi.SettlementsResponse{
		TotalSettlementReceived: bigint.Wrap(big.NewInt(15000)),
		TotalSettlementSent:     bigint.Wrap(big.NewInt(80000)),
		Settlements: []debugapi.SettlementResponse{
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
	var got *debugapi.SettlementsResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !equalSettlements(got, expected) {
		t.Errorf("got settlements: %+v, expected: %+v", got, expected)
	}

}

func TestSettlementsError(t *testing.T) {
	wantErr := errors.New("New errors")
	settlementsSentFunc := func() (map[string]*big.Int, error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		SettlementOpts: []mock.Option{mock.WithSettlementsSentFunc(settlementsSentFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrCantSettlements,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestSettlementsPeers(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	settlementSentFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(1000000000000000000), nil
	}
	testServer := newTestServer(t, testServerOptions{
		SettlementOpts: []mock.Option{mock.WithSettlementSentFunc(settlementSentFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements/"+peer, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.SettlementResponse{
			Peer:               peer,
			SettlementSent:     bigint.Wrap(big.NewInt(1000000000000000000)),
			SettlementReceived: bigint.Wrap(big.NewInt(0)),
		}),
	)
}

func TestSettlementsPeersNoSettlements(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	noErrFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(1000000000000000000), nil
	}
	errFunc := func(swarm.Address) (*big.Int, error) {
		return nil, settlement.ErrPeerNoSettlements
	}

	t.Run("no sent", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			SettlementOpts: []mock.Option{
				mock.WithSettlementSentFunc(errFunc),
				mock.WithSettlementRecvFunc(noErrFunc),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements/"+peer, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.SettlementResponse{
				Peer:               peer,
				SettlementSent:     bigint.Wrap(big.NewInt(0)),
				SettlementReceived: bigint.Wrap(big.NewInt(1000000000000000000)),
			}),
		)
	})

	t.Run("no received", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			SettlementOpts: []mock.Option{
				mock.WithSettlementSentFunc(noErrFunc),
				mock.WithSettlementRecvFunc(errFunc),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements/"+peer, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.SettlementResponse{
				Peer:               peer,
				SettlementSent:     bigint.Wrap(big.NewInt(1000000000000000000)),
				SettlementReceived: bigint.Wrap(big.NewInt(0)),
			}),
		)
	})
}

func TestSettlementsPeersError(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	settlementSentFunc := func(swarm.Address) (*big.Int, error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		SettlementOpts: []mock.Option{mock.WithSettlementSentFunc(settlementSentFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements/"+peer, http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrCantSettlementsPeer,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestSettlementsInvalidAddress(t *testing.T) {
	peer := "bad peer address"

	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements/"+peer, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrInvalidAddress,
			Code:    http.StatusNotFound,
		}),
	)
}

func equalSettlements(a, b *debugapi.SettlementsResponse) bool {
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
