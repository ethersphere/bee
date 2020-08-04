// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"errors"
	"github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/swarm"
	"net/http"
	"reflect"
	"testing"
)

func TestBalancesOK(t *testing.T) {
	balancesFunc := func() (ret map[string]int64, err error) {
		ret = make(map[string]int64)
		ret["DEAD"] = 1000000000000000000
		ret["BEEF"] = -100000000000000000
		ret["PARTY"] = 0
		return ret, err
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalancesFunc(balancesFunc)},
	})

	expected := &debugapi.BalancesResponse{
		[]debugapi.BalanceResponse{
			{
				Peer:    "DEAD",
				Balance: 1000000000000000000,
			},
			{
				Peer:    "BEEF",
				Balance: -100000000000000000,
			},
			{
				Peer:    "PARTY",
				Balance: 0,
			},
		},
	}

	// We expect a list of items unordered by peer:
	got := jsonhttptest.ResponseReturnDirect(t, testServer.Client, http.MethodGet, "/balances", nil, http.StatusOK, &debugapi.BalancesResponse{
		[]debugapi.BalanceResponse{
			{
				Peer:    "J",
				Balance: 5,
			},
		},
	}).(*debugapi.BalancesResponse)

	if !comparisonOfBalances(got, expected) {
		t.Errorf("Workin got %v, not as expected %v", got, expected)
	}

}

func TestBalancesError(t *testing.T) {
	wantErr := errors.New("ASDF")
	balancesFunc := func() (ret map[string]int64, err error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalancesFunc(balancesFunc)},
	})

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/balances", nil, http.StatusInternalServerError, jsonhttp.StatusResponse{
		Message: debugapi.ErrCantBalances,
		Code:    500,
	})
}

func TestBalancesPeersOK(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	balanceFunc := func(swarm.Address) (int64, error) {
		return 1000000000000000000, nil
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/balances/"+peer, nil, http.StatusOK, debugapi.BalanceResponse{
		Peer:    peer,
		Balance: 1000000000000000000,
	})
}

func TestBalancesPeersError(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	balanceFunc := func(swarm.Address) (int64, error) {
		return 0, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/balances/"+peer, nil, http.StatusInternalServerError, jsonhttp.StatusResponse{
		Message: wantErr.Error(),
		Code:    http.StatusInternalServerError,
	})
}

func TestMalformedPeer(t *testing.T) {
	peer := "bad peer address"
	wantErr := "malformed peer address"

	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/balances/"+peer, nil, http.StatusBadRequest, jsonhttp.StatusResponse{
		Message: wantErr,
		Code:    http.StatusBadRequest,
	})
}

func comparisonOfBalances(a, b *debugapi.BalancesResponse) bool {

	var state bool

	for akeys := range a.Balances {
		state = false
		for bkeys := range b.Balances {
			if reflect.DeepEqual(a.Balances[akeys], b.Balances[bkeys]) {
				state = true
			}
		}
		if !state {
			return false
		}
	}

	for bkeys := range b.Balances {

		state = false
		for akeys := range a.Balances {
			if reflect.DeepEqual(a.Balances[akeys], b.Balances[bkeys]) {
				state = true
			}
		}
		if !state {
			return false
		}
	}

	return true
}
