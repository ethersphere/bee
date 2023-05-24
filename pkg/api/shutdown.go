// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

const (
	msgShutdownStarted        = "shutdown procedure has started"
	msgShutdownAlreadyStarted = "shutdown procedure already started"
)

func (s *Service) shutdownHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("shutdown").Build()

	var msg string

	if s.shutdownStarted.Swap(true) {
		msg = msgShutdownAlreadyStarted
	} else {
		go s.gracefulShutdown()
		msg = msgShutdownStarted
	}

	logger.Debug(msg)
	jsonhttp.OK(w, msg)
}

func (s *Service) gracefulShutdown() {
	// TODO: wait on components to gracefully close
	// <-s.redistributionAgent.Halt()

	// Signal when shutdown is ready
	s.shutdownSig.Signal()
}
