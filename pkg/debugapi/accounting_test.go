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
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestAccountingInfo(t *testing.T) {
	accountingFunc := func() (map[string]accounting.PeerInfo, error) {
		ret := make(map[string]accounting.PeerInfo)
		ret["BEEF"] = accounting.PeerInfo{
			Balance:               big.NewInt(25),
			ThresholdReceived:     big.NewInt(37),
			ThresholdGiven:        big.NewInt(49),
			SurplusBalance:        big.NewInt(74),
			ReservedBalance:       big.NewInt(85),
			ShadowReservedBalance: big.NewInt(92),
			GhostBalance:          big.NewInt(94),
		}
		ret["B33F"] = accounting.PeerInfo{
			Balance:               big.NewInt(26),
			ThresholdReceived:     big.NewInt(38),
			ThresholdGiven:        big.NewInt(50),
			SurplusBalance:        big.NewInt(75),
			ReservedBalance:       big.NewInt(86),
			ShadowReservedBalance: big.NewInt(93),
			GhostBalance:          big.NewInt(95),
		}
		ret["BE3F"] = accounting.PeerInfo{
			Balance:               big.NewInt(27),
			ThresholdReceived:     big.NewInt(39),
			ThresholdGiven:        big.NewInt(51),
			SurplusBalance:        big.NewInt(76),
			ReservedBalance:       big.NewInt(87),
			ShadowReservedBalance: big.NewInt(94),
			GhostBalance:          big.NewInt(96),
		}

		return ret, nil
	}

	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithPeerAccountingFunc(accountingFunc)},
	})

	expected := &debugapi.PeerData{
		InfoResponse: map[string]debugapi.PeerDataResponse{
			"BEEF": debugapi.PeerDataResponse{
				Balance:               bigint.Wrap(big.NewInt(25)),
				ThresholdReceived:     bigint.Wrap(big.NewInt(37)),
				ThresholdGiven:        bigint.Wrap(big.NewInt(49)),
				SurplusBalance:        bigint.Wrap(big.NewInt(74)),
				ReservedBalance:       bigint.Wrap(big.NewInt(85)),
				ShadowReservedBalance: bigint.Wrap(big.NewInt(92)),
				GhostBalance:          bigint.Wrap(big.NewInt(94)),
			},
			"B33F": debugapi.PeerDataResponse{
				Balance:               bigint.Wrap(big.NewInt(26)),
				ThresholdReceived:     bigint.Wrap(big.NewInt(38)),
				ThresholdGiven:        bigint.Wrap(big.NewInt(50)),
				SurplusBalance:        bigint.Wrap(big.NewInt(75)),
				ReservedBalance:       bigint.Wrap(big.NewInt(86)),
				ShadowReservedBalance: bigint.Wrap(big.NewInt(93)),
				GhostBalance:          bigint.Wrap(big.NewInt(95)),
			},
			"BE3F": debugapi.PeerDataResponse{
				Balance:               bigint.Wrap(big.NewInt(27)),
				ThresholdReceived:     bigint.Wrap(big.NewInt(39)),
				ThresholdGiven:        bigint.Wrap(big.NewInt(51)),
				SurplusBalance:        bigint.Wrap(big.NewInt(76)),
				ReservedBalance:       bigint.Wrap(big.NewInt(87)),
				ShadowReservedBalance: bigint.Wrap(big.NewInt(94)),
				GhostBalance:          bigint.Wrap(big.NewInt(96)),
			},
		},
	}

	// We expect a list of items unordered by peer:
	var got *debugapi.PeerData
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/accounting", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got accounting: %v, expected: %v", got, expected)
	}

}

func TestAccountingInfoError(t *testing.T) {
	wantErr := errors.New("ASDF")
	accountingFunc := func() (map[string]accounting.PeerInfo, error) {
		return nil, wantErr
	}
	testServer := newTestServer(t, testServerOptions{
		AccountingOpts: []mock.Option{mock.WithPeerAccountingFunc(accountingFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/accounting", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrCantInfo,
			Code:    http.StatusInternalServerError,
		}),
	)
}
