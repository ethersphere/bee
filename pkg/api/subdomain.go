// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/mux"
)

func (s *Service) subdomainHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_subdomain").Build())

	paths := struct {
		Subdomain string `map:"subdomain" validate:"required"`
		Path      string `map:"path"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}
	if strings.HasSuffix(paths.Path, "/") {
		paths.Path = strings.TrimRight(paths.Path, "/") + "/" // NOTE: leave one slash if there was some.
	}

	address, err := s.resolveNameOrAddress(paths.Subdomain)
	if err != nil {
		logger.Debug("subdomain get: mapStructure address string failed", "string", paths.Subdomain, "error", err)
		logger.Error(nil, "subdomain get: mapStructure address string failed")
		jsonhttp.NotFound(w, nil)
		return
	}

	s.serveReference(logger, address, paths.Path, w, r)
}
