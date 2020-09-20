// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"math/big"
	"net/http"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock"
)

func TestChequebookBalance(t *testing.T) {
	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		ret = big.NewInt(9000)
		return ret, nil
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookBalanceFunc(chequebookBalanceFunc)},
	})

	expected := &debugapi.ChequebookBalanceResponse{
		Balance: big.NewInt(9000),
	}
	// We expect a list of items unordered by peer:
	var got *debugapi.ChequebookBalanceResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/balance", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got settlements: %+v, expected: %+v", got, expected)
	}

}
