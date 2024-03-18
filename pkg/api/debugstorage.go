// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/tracing"
)

func (s *Service) debugStorage(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("debug_storage").Build())

	info, err := s.storer.DebugInfo(r.Context())
	if err != nil {
		logger.Debug("get debug storage info failed", "error", err)
		logger.Error(nil, "get debug storage info failed")
		jsonhttp.InternalServerError(w, "debug storage info not available")
		return
	}

	jsonhttp.OK(w, info)
}
