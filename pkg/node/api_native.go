// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !js

package node

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
)

// startAPIServer starts the API server for native builds using TCP listener.
func startAPIServer(addr string, handler http.Handler, errorLogWriter io.Writer, logger log.Logger) (*http.Server, io.Closer, error) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("api listener: %w", err)
	}

	server := &http.Server{
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           handler,
		ErrorLog:          stdlog.New(errorLogWriter, "", 0),
	}

	go func() {
		logger.Info("starting debug & api server", "address", listener.Addr())

		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Debug("debug & api server failed to start", "error", err)
			logger.Error(nil, "debug & api server failed to start")
		}
	}()

	return server, server, nil
}
