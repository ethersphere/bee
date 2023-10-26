// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/tracing"
)

func (s *Service) debugStorageHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_debug_storage").Build())

	info, err := s.storer.DebugInfo(r.Context())
	if err != nil {
		logger.Debug("unable to get debug storage info", "error", err)
		logger.Warning("unable to get debug storage info")
		jsonhttp.InternalServerError(w, "debug store info")
		return
	}

	jsonhttp.OK(w, info)
}
