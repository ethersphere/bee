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

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSettlements(t *testing.T) {
	settlementsSentFunc := func() (ret map[string]uint64, err error) {
		ret = make(map[string]uint64)
		ret["DEAD"] = 10000
		ret["BEEF"] = 20000
		ret["FFFF"] = 50000
		return ret, err
	}

	settlementsRecvFunc := func() (ret map[string]uint64, err error) {
		ret = make(map[string]uint64)
		ret["BEEF"] = 10000
		ret["EEEE"] = 5000
		return ret, err
	}

	testServer := newTestServer(t, testServerOptions{
		SettlementOpts: []mock.Option{mock.WithSettlementsSentFunc(settlementsSentFunc), mock.WithSettlementsRecvFunc(settlementsRecvFunc)},
	})

	expected := &debugapi.SettlementsResponse{
		TotalSettlementReceived: big.NewInt(15000),
		TotalSettlementSent:     big.NewInt(80000),
		Settlements: []debugapi.SettlementResponse{
			{
				Peer:               "DEAD",
				SettlementReceived: 0,
				SettlementSent:     10000,
			},
			{
				Peer:               "BEEF",
				SettlementReceived: 10000,
				SettlementSent:     20000,
			},
			{
				Peer:               "FFFF",
				SettlementReceived: 0,
				SettlementSent:     50000,
			},
			{
				Peer:               "EEEE",
				SettlementReceived: 5000,
				SettlementSent:     0,
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
	settlementsSentFunc := func() (map[string]uint64, error) {
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
	settlementSentFunc := func(swarm.Address) (uint64, error) {
		return 1000000000000000000, nil
	}
	testServer := newTestServer(t, testServerOptions{
		SettlementOpts: []mock.Option{mock.WithSettlementSentFunc(settlementSentFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/settlements/"+peer, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.SettlementResponse{
			Peer:               peer,
			SettlementSent:     1000000000000000000,
			SettlementReceived: 0,
		}),
	)
}

func TestSettlementsPeersError(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	settlementSentFunc := func(swarm.Address) (uint64, error) {
		return 0, wantErr
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
			Message: debugapi.ErrInvaliAddress,
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

	if a.TotalSettlementReceived.Cmp(b.TotalSettlementReceived) != 0 {
		return false
	}

	if a.TotalSettlementSent.Cmp(b.TotalSettlementSent) != 0 {
		return false
	}

	return true
}
