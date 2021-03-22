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

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestBalances(t *testing.T) {
	compensatedBalancesFunc := func() (ret map[string]*big.Int, err error) {
		ret = make(map[string]*big.Int)
		ret["DEAD"] = big.NewInt(1000000000000000000)
		ret["BEEF"] = big.NewInt(-100000000000000000)
		ret["PARTY"] = big.NewInt(0)
		return ret, err
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalancesFunc(compensatedBalancesFunc)},
	})

	expected := &debugapi.BalancesResponse{
		[]debugapi.BalanceResponse{
			{
				Peer:    "DEAD",
				Balance: big.NewInt(1000000000000000000),
			},
			{
				Peer:    "BEEF",
				Balance: big.NewInt(-100000000000000000),
			},
			{
				Peer:    "PARTY",
				Balance: big.NewInt(0),
			},
		},
	}

	// We expect a list of items unordered by peer:
	var got *debugapi.BalancesResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/balances", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !equalBalances(got, expected) {
		t.Errorf("got balances: %v, expected: %v", got, expected)
	}

}

func TestBalancesError(t *testing.T) {
	wantErr := errors.New("ASDF")
	compensatedBalancesFunc := func() (ret map[string]*big.Int, err error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalancesFunc(compensatedBalancesFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/balances", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrCantBalances,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestBalancesPeers(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	compensatedBalanceFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(100000000000000000), nil
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalanceFunc(compensatedBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/balances/"+peer, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.BalanceResponse{
			Peer:    peer,
			Balance: big.NewInt(100000000000000000),
		}),
	)
}

func TestBalancesPeersError(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	compensatedBalanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalanceFunc(compensatedBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/balances/"+peer, http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrCantBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestBalancesPeersNoBalance(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	compensatedBalanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, accounting.ErrPeerNoBalance
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalanceFunc(compensatedBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/balances/"+peer, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrNoBalance,
			Code:    http.StatusNotFound,
		}),
	)
}

func TestBalancesInvalidAddress(t *testing.T) {
	peer := "bad peer address"

	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/balances/"+peer, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrInvalidAddress,
			Code:    http.StatusNotFound,
		}),
	)
}

func equalBalances(a, b *debugapi.BalancesResponse) bool {
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

func TestConsumedBalances(t *testing.T) {
	balancesFunc := func() (ret map[string]*big.Int, err error) {
		ret = make(map[string]*big.Int)
		ret["DEAD"] = big.NewInt(1000000000000000000)
		ret["BEEF"] = big.NewInt(-100000000000000000)
		ret["PARTY"] = big.NewInt(0)
		return ret, err
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalancesFunc(balancesFunc)},
	})

	expected := &debugapi.BalancesResponse{
		[]debugapi.BalanceResponse{
			{
				Peer:    "DEAD",
				Balance: big.NewInt(1000000000000000000),
			},
			{
				Peer:    "BEEF",
				Balance: big.NewInt(-100000000000000000),
			},
			{
				Peer:    "PARTY",
				Balance: big.NewInt(0),
			},
		},
	}

	// We expect a list of items unordered by peer:
	var got *debugapi.BalancesResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/consumed", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !equalBalances(got, expected) {
		t.Errorf("got balances: %v, expected: %v", got, expected)
	}

}

func TestConsumedError(t *testing.T) {
	wantErr := errors.New("ASDF")
	balancesFunc := func() (ret map[string]*big.Int, err error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalancesFunc(balancesFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/consumed", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrCantBalances,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestConsumedPeers(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	balanceFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(1000000000000000000), nil
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/consumed/"+peer, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.BalanceResponse{
			Peer:    peer,
			Balance: big.NewInt(1000000000000000000),
		}),
	)
}

func TestConsumedPeersError(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	balanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/consumed/"+peer, http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrCantBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestConsumedPeersNoBalance(t *testing.T) {
	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	balanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, accounting.ErrPeerNoBalance
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/consumed/"+peer, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrNoBalance,
			Code:    http.StatusNotFound,
		}),
	)
}

func TestConsumedInvalidAddress(t *testing.T) {
	peer := "bad peer address"

	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/consumed/"+peer, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrInvalidAddress,
			Code:    http.StatusNotFound,
		}),
	)
}
