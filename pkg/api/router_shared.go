// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"resenje.org/web"
)

const (
	apiVersion = "v1" // Only one api version exists, this should be configurable with more.
	rootPath   = "/" + apiVersion
)

func (s *Service) Mount() {
	if s == nil {
		return
	}

	router := mux.NewRouter()

	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	s.router = router

	s.mountTechnicalDebug()
	s.mountBusinessDebug()
	s.mountAPI()

	s.Handler = web.ChainHandlers(
		httpaccess.NewHTTPAccessLogHandler(s.logger, s.tracer, "api access"),
		handlers.CompressHandler,
		s.corsHandler,
		web.NoCacheHeadersHandler,
		web.FinalHandler(router),
	)
}

func (s *Service) checkRouteAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.fullAPIEnabled {
			jsonhttp.ServiceUnavailable(w, "Node is syncing. This endpoint is unavailable. Try again later.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Service) checkSwapAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.swapEnabled {
			jsonhttp.NotImplemented(w, "Swap is disabled. This endpoint is unavailable.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Service) checkChequebookAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.chequebookEnabled {
			jsonhttp.NotImplemented(w, "Chequebook is disabled. This endpoint is unavailable.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}
