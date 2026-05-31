// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build js

package node

import (
	"context"
	"io"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/log"
	wasmhttp "github.com/nlepage/go-wasm-http-server/v2"
)

// startAPIServer starts the API server for wasm builds. In the browser there is
// no TCP listener; instead wasmhttp registers the handler with the service
// worker's fetch handler. There is no graceful shutdown, so the returned server
// and closer are nil (node shutdown nil-guards both).
func startAPIServer(_ context.Context, _ string, handler http.Handler, _ io.Writer, logger log.Logger) (*http.Server, io.Closer, error) {
	logger.Info("starting debug & api server via service worker")
	wasmhttp.Serve(handler)
	return nil, nil, nil
}
