// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestOverlay(t *testing.T) {
	address := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")

	testServer := newTestServer(t, testServerOptions{
		Overlay: address,
	})
	defer testServer.Cleanup()

	jsonhttptest.ResponseDirect(t, testServer.Client, http.MethodGet, "/overlay-address", nil, http.StatusOK, debugapi.OverlayAddressResponse{
		Address: address,
	})
}
