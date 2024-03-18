// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
)

const pinRef = "620fcd78c7ce54da2d1b7cc2274a02e190cbe8fecbc3bd244690ab6517ce8f39"

func TestIntegrityHandler(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI: true,
			PinIntegrity: &mockPinIntegrity{
				Store:  inmemstore.New(),
				tester: t,
			},
		})

		endp := "/check/pin?ref=" + pinRef

		// When probe is not set health endpoint should indicate that node is not healthy
		jsonhttptest.Request(t, testServer, http.MethodGet, endp, http.StatusOK, jsonhttptest.WithExpectedResponse(nil))
	})

	t.Run("wrong hash format", func(t *testing.T) {
		t.Parallel()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI: true,
			PinIntegrity: &mockPinIntegrity{
				Store:  inmemstore.New(),
				tester: t,
			},
		})

		endp := "/check/pin?ref=0xbadhash"

		// When probe is not set health endpoint should indicate that node is not healthy
		jsonhttptest.Request(t, testServer, http.MethodGet, endp, http.StatusBadRequest, jsonhttptest.WithExpectedResponse(nil))
	})
}

type mockPinIntegrity struct {
	tester *testing.T
	Store  storage.Store
}

func (p *mockPinIntegrity) Check(ctx context.Context, logger log.Logger, pin string, out chan storer.PinStat) {
	if pin != pinRef {
		p.tester.Fatal("bad pin", pin)
	}
	close(out)
}
