// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging/httpaccess"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	const (
		apiVersion = "v1" // Only one api version exists, this should be configurable with more.
		rootPath   = "/" + apiVersion
	)

	router := mux.NewRouter()

	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})

	router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	router.HandleFunc("/{protocol}/{rest:.*}", s.engine)

	s.Handler = web.ChainHandlers(
		httpaccess.NewHTTPAccessLogHandler(s.logger, logrus.InfoLevel, s.tracer, "api access"),
		handlers.CompressHandler,
		// todo: add recovery handler
		s.responseCodeMetricsHandler,
		s.pageviewMetricsHandler,
		func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if o := r.Header.Get("Origin"); o != "" && s.checkOrigin(r) {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Origin", o)
					w.Header().Set("Access-Control-Allow-Headers", "User-Agent, Origin, Accept, Authorization, Content-Type, X-Requested-With, Decompressed-Content-Length, Access-Control-Request-Headers, Access-Control-Request-Method, Swarm-Tag, Swarm-Pin, Swarm-Encrypt, Swarm-Index-Document, Swarm-Error-Document, Swarm-Collection, Swarm-Postage-Batch-Id, Gas-Price")
					w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, DELETE")
					w.Header().Set("Access-Control-Max-Age", "3600")
				}
				h.ServeHTTP(w, r)
			})
		},
		s.gatewayModeForbidHeadersHandler,
		web.FinalHandler(router),
	)
}

func (s *server) gatewayModeForbidEndpointHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.GatewayMode {
			s.logger.Tracef("gateway mode: forbidden %s", r.URL.String())
			jsonhttp.Forbidden(w, nil)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *server) gatewayModeForbidHeadersHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.GatewayMode {
			if strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true" {
				s.logger.Tracef("gateway mode: forbidden pinning %s", r.URL.String())
				jsonhttp.Forbidden(w, "pinning is disabled")
				return
			}
			if strings.ToLower(r.Header.Get(SwarmEncryptHeader)) == "true" {
				s.logger.Tracef("gateway mode: forbidden encryption %s", r.URL.String())
				jsonhttp.Forbidden(w, "encryption is disabled")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
