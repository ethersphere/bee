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
	logger := tracing.NewRootLoggerWithTraceID(r.Context(), s.logger)

	nameOrHex := mux.Vars(r)["subdomain"]
	pathVar := mux.Vars(r)["path"]
	if strings.HasSuffix(pathVar, "/") {
		pathVar = strings.TrimRight(pathVar, "/")
		// NOTE: leave one slash if there was some
		pathVar += "/"
	}

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debug("subdomain get: parse address string failed", "string", nameOrHex, "error", err)
		logger.Error(nil, "subdomain get: parse address string failed")
		jsonhttp.NotFound(w, nil)
		return
	}

	s.serveReference(address, pathVar, w, r)
}
