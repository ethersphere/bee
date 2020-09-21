// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock"
)

func TestChequebookBalance(t *testing.T) {
	returnedBalance := big.NewInt(9000)

	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {

		return returnedBalance, nil
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookBalanceFunc(chequebookBalanceFunc)},
	})

	expected := &debugapi.ChequebookBalanceResponse{
		Balance: returnedBalance,
	}
	// We expect a list of items unordered by peer:
	var got *debugapi.ChequebookBalanceResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/balance", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got balance: %+v, expected: %+v", got, expected)
	}

}

func TestChequebookBalanceError(t *testing.T) {
	wantErr := errors.New("New errors")
	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return big.NewInt(0), wantErr
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookBalanceFunc(chequebookBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/balance", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrChequebookBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestChequebookAddress(t *testing.T) {
	chequebookAddressFunc := func() common.Address {
		return common.HexToAddress("0xfffff")
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookAddressFunc(chequebookAddressFunc)},
	})

	address := common.HexToAddress("0xfffff")

	expected := &debugapi.ChequebookAddressResponse{
		Address: address.String(),
	}

	var got *debugapi.ChequebookAddressResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/address", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got address: %+v, expected: %+v", got, expected)
	}

}
