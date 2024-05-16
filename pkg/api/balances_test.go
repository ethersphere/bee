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

	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestBalances(t *testing.T) {
	t.Parallel()

	compensatedBalancesFunc := func() (ret map[string]*big.Int, err error) {
		ret = make(map[string]*big.Int)
		ret["DEAD"] = big.NewInt(1000000000000000000)
		ret["BEEF"] = big.NewInt(-100000000000000000)
		ret["PARTY"] = big.NewInt(0)
		return ret, err
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalancesFunc(compensatedBalancesFunc)},
	})

	expected := &api.BalancesResponse{
		[]api.BalanceResponse{
			{
				Peer:    "DEAD",
				Balance: bigint.Wrap(big.NewInt(1000000000000000000)),
			},
			{
				Peer:    "BEEF",
				Balance: bigint.Wrap(big.NewInt(-100000000000000000)),
			},
			{
				Peer:    "PARTY",
				Balance: bigint.Wrap(big.NewInt(0)),
			},
		},
	}

	// We expect a list of items unordered by peer:
	var got *api.BalancesResponse
	jsonhttptest.Request(t, testServer, http.MethodGet, "/balances", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !equalBalances(got, expected) {
		t.Errorf("got balances: %v, expected: %v", got, expected)
	}
}

func TestBalancesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("ASDF")
	compensatedBalancesFunc := func() (ret map[string]*big.Int, err error) {
		return nil, wantErr
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalancesFunc(compensatedBalancesFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/balances", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrCantBalances,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestBalancesPeers(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	compensatedBalanceFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(100000000000000000), nil
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalanceFunc(compensatedBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/balances/"+peer, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(api.BalanceResponse{
			Peer:    peer,
			Balance: bigint.Wrap(big.NewInt(100000000000000000)),
		}),
	)
}

func TestBalancesPeersError(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	compensatedBalanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, wantErr
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalanceFunc(compensatedBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/balances/"+peer, http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrCantBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestBalancesPeersNoBalance(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	compensatedBalanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, accounting.ErrPeerNoBalance
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithCompensatedBalanceFunc(compensatedBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/balances/"+peer, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrNoBalance,
			Code:    http.StatusNotFound,
		}),
	)
}

func equalBalances(a, b *api.BalancesResponse) bool {
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
	t.Parallel()

	balancesFunc := func() (ret map[string]*big.Int, err error) {
		ret = make(map[string]*big.Int)
		ret["DEAD"] = big.NewInt(1000000000000000000)
		ret["BEEF"] = big.NewInt(-100000000000000000)
		ret["PARTY"] = big.NewInt(0)
		return ret, err
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalancesFunc(balancesFunc)},
	})

	expected := &api.BalancesResponse{
		[]api.BalanceResponse{
			{
				Peer:    "DEAD",
				Balance: bigint.Wrap(big.NewInt(1000000000000000000)),
			},
			{
				Peer:    "BEEF",
				Balance: bigint.Wrap(big.NewInt(-100000000000000000)),
			},
			{
				Peer:    "PARTY",
				Balance: bigint.Wrap(big.NewInt(0)),
			},
		},
	}

	// We expect a list of items unordered by peer:
	var got *api.BalancesResponse
	jsonhttptest.Request(t, testServer, http.MethodGet, "/consumed", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !equalBalances(got, expected) {
		t.Errorf("got balances: %v, expected: %v", got, expected)
	}

}

func TestConsumedError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("ASDF")
	balancesFunc := func() (ret map[string]*big.Int, err error) {
		return nil, wantErr
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalancesFunc(balancesFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/consumed", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrCantBalances,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestConsumedPeers(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	balanceFunc := func(swarm.Address) (*big.Int, error) {
		return big.NewInt(1000000000000000000), nil
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/consumed/"+peer, http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(api.BalanceResponse{
			Peer:    peer,
			Balance: bigint.Wrap(big.NewInt(1000000000000000000)),
		}),
	)
}

func TestConsumedPeersError(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	wantErr := errors.New("Error")
	balanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, wantErr
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/consumed/"+peer, http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrCantBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestConsumedPeersNoBalance(t *testing.T) {
	t.Parallel()

	peer := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	balanceFunc := func(swarm.Address) (*big.Int, error) {
		return nil, accounting.ErrPeerNoBalance
	}
	testServer, _, _, _ := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithBalanceFunc(balanceFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/consumed/"+peer, http.StatusNotFound,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrNoBalance,
			Code:    http.StatusNotFound,
		}),
	)
}

func Test_peerBalanceHandler_invalidInputs(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/consumed/"+tc.peer, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func Test_compensatedPeerBalanceHandler_invalidInputs(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/balances/"+tc.peer, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}
