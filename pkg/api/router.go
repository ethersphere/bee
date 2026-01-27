// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendnoop"
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

// EnableFullAPI will enable all available endpoints, because some endpoints are not available during syncing.
func (s *Service) EnableFullAPI() {
	if s == nil {
		return
	}

	s.fullAPIEnabled = true

	compressHandler := func(h http.Handler) http.Handler {
		downloadEndpoints := []string{
			"/bzz",
			"/bytes",
			"/chunks",
			"/feeds",
			"/soc",
			rootPath + "/bzz",
			rootPath + "/bytes",
			rootPath + "/chunks",
			rootPath + "/feeds",
			rootPath + "/soc",
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip compression for GET requests on download endpoints.
			// This is done in order to preserve Content-Length header in response,
			// because CompressHandler is always removing it.
			if r.Method == http.MethodGet {
				for _, endpoint := range downloadEndpoints {
					if strings.HasPrefix(r.URL.Path, endpoint) {
						h.ServeHTTP(w, r)
						return
					}
				}
			}

			if r.Method == http.MethodHead {
				h.ServeHTTP(w, r)
				return
			}

			handlers.CompressHandler(h).ServeHTTP(w, r)
		})
	}

	s.Handler = web.ChainHandlers(
		httpaccess.NewHTTPAccessLogHandler(s.logger, s.tracer, "api access"),
		compressHandler,
		s.responseCodeMetricsHandler,
		s.pageviewMetricsHandler,
		s.corsHandler,
		web.FinalHandler(s.router),
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
			jsonhttp.Forbidden(w, "Swap is disabled. This endpoint is unavailable.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Service) checkChequebookAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.chequebookEnabled {
			jsonhttp.Forbidden(w, "Chequebook is disabled. This endpoint is unavailable.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Service) checkChainAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.chainBackend.(*backendnoop.Backend); ok {
			jsonhttp.Forbidden(w, "Chain is disabled. This endpoint is unavailable.")
			return
		}

		handler.ServeHTTP(w, r)
	})
}
