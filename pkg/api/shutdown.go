// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import "net/http"

func (s *Service) shutdownHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("shutdown").Build()

	if s.shutdownStarted.Swap(true) {
		logger.Debug("shutdown procedure already started")
	} else {
		go s.gracefulShutdown()
		logger.Debug("shutdown procedure has started")
	}
}

func (s *Service) gracefulShutdown() {
	// TODO: wait on components to gracefully close
	// <-s.redistributionAgent.Halt()

	// Signal when shutdown is ready
	s.shutdownSig.Signal()
}
