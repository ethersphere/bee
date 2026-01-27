// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build js && wasm

package node

import (
	"io"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/log"
	wasmhttp "github.com/nlepage/go-wasm-http-server/v2"
)

// startAPIServer starts the API server for WASM builds.
// In WASM, we use wasmhttp which hooks into the service worker's fetch handler.
// Returns nil for server and closer since WASM doesn't need graceful shutdown.
func startAPIServer(_ string, handler http.Handler, _ io.Writer, logger log.Logger) (*http.Server, io.Closer, error) {
	logger.Info("starting debug & api server via service worker")

	// wasmhttp.Serve registers the handler with the service worker
	// by calling self.wasmhttp.setHandler in JavaScript
	wasmhttp.Serve(handler)

	return nil, nil, nil
}
