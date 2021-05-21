// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
)

func TestReserveState(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(mock.WithReserveState(&postage.ReserveState{
				Radius: 5,
			})),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&postage.ReserveState{
				Radius: 5,
			}),
		)
	})
	t.Run("empty", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&postage.ReserveState{}),
		)
	})
}

func TestChainState(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		cs := &postage.ChainState{
			Block:        123456,
			TotalAmount:  big.NewInt(50),
			CurrentPrice: big.NewInt(5),
		}
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(mock.WithChainState(cs)),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/chainstate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(cs),
		)
	})

	t.Run("empty", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/chainstate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&postage.ChainState{}),
		)
	})
}
