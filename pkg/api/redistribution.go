// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/tracing"
	"net/http"
)

func (s *Service) redistributionStatusHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("redistribution_status").Build())

	status, err := s.redistributionAgent.GetStatus()
	if err != nil {
		logger.Debug("get redistribution status", "overlay address", s.overlay.String(), "error", err)
		logger.Error(nil, "get redistribution status")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	jsonhttp.OK(w, status)
}
