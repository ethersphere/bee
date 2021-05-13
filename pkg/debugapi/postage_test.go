// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
)

func TestReservestate(t *testing.T) {

	ts := newTestServer(t, testServerOptions{
		BatchStore: mock.New(mock.WithReserveState(&postage.Reservestate{
			Radius: 5,
			Outer:  big.NewInt(5),
			Inner:  big.NewInt(5),
		})),
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.ReserveStateResponse{
				Radius: 5,
				Outer:  bigint.Wrap(big.NewInt(5)),
				Inner:  bigint.Wrap(big.NewInt(5)),
			}),
		)
	})
}
