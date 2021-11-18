// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/auth"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging/httpaccess"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/handlers"
	"github.com/sirupsen/logrus"
)

func (s *server) setupRouting() {
	const (
		apiVersion = "v1" // Only one api version exists, this should be configurable with more.
		rootPath   = "/" + apiVersion
	)

	r := chi.NewRouter()

	r.Use(
		jsonhttp.RecovererMiddleware(s.logger),
		httpaccess.NewHTTPAccessLogHandler(s.logger, logrus.InfoLevel, s.tracer, "api access"),
		handlers.CompressHandler,
		s.responseCodeMetricsHandler,
		s.pageviewMetricsHandler,
		s.originMiddleware,
		s.gatewayModeForbidHeadersHandler,
	)

	r.NotFound(jsonhttp.NotFoundHandler)
	r.MethodNotAllowed(jsonhttp.MethodNotAllowedHandler)

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})
	r.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	api := chi.NewRouter()

	if s.Restricted {
		r.With(s.newTracingHandler("auth"), jsonhttp.NewMaxBodyBytesHandler(512)).Post("/auth", s.authHandler)
		r.With(s.newTracingHandler("auth"), jsonhttp.NewMaxBodyBytesHandler(512)).Post("/refresh", s.refreshHandler)
		api.Use(auth.PermissionCheckHandler(s.auth))
	}

	api.With(s.contentLengthMetricMiddleware(), s.newTracingHandler("bytes-upload")).Post("/bytes", s.bytesUploadHandler)
	api.With(s.contentLengthMetricMiddleware(), s.newTracingHandler("bytes-download")).Get("/bytes/{address}", s.bytesGetHandler)

	api.With(jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize)).Post("/chunks", s.chunkUploadHandler)
	api.Get("/chunks/{addr}", s.chunkGetHandler)
	api.With(s.newTracingHandler("chunks-stream-upload")).HandleFunc("/chunks/stream", s.chunkUploadStreamHandler)

	api.With(jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize)).Post("/soc/{owner}/{id}", s.socUploadHandler)

	api.Get("/feeds/{owner}/{topic}", s.feedGetHandler)
	api.With(jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize)).Post("/feeds/{owner}/{topic}", s.feedPostHandler)

	api.HandleFunc("/bzz/{address}", func(w http.ResponseWriter, r *http.Request) {
		u := r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
	})
	api.With(s.contentLengthMetricMiddleware(), s.newTracingHandler("bzz-upload")).Post("/bzz", s.bzzUploadHandler)
	api.With(s.contentLengthMetricMiddleware(), s.newTracingHandler("bzz-download")).Get("/bzz/{address}/*", s.bzzDownloadHandler)
	api.With(s.newTracingHandler("bzz-patch")).Patch("/bzz/{address}/*", s.bzzPatchHandler)

	// gateway forbidden
	api.Group(func(r chi.Router) {

		r.Use(s.gatewayModeForbidEndpointHandler)

		r.With(jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkSize)).Post("/pss/send/{topic}/{targets}", s.pssPostHandler)
		r.HandleFunc("/pss/subscribe/{topic}", s.pssWsHandler)

		r.Get("/tags", s.listTagsHandler)
		r.With(jsonhttp.NewMaxBodyBytesHandler(1024)).Post("/tags", s.createTagHandler)

		r.Get("/tags/{id}", s.getTagHandler)
		r.Delete("/tags/{id}", s.deleteTagHandler)
		r.With(jsonhttp.NewMaxBodyBytesHandler(1024)).Patch("/tags/{id}", s.doneSplitHandler)

		r.Get("/pins", s.listPinnedRootHashes)
		r.Get("/pins/{reference}", s.getPinnedRootHash)
		r.Post("/pins/{reference}", s.pinRootHash)
		r.Delete("/pins/{reference}", s.unpinRootHash)

		r.Get("/stewardship/{address}", s.stewardshipGetHandler)
		r.Put("/stewardship/{address}", s.stewardshipPutHandler)
	})

	r.Mount("/", api)
	r.Mount(rootPath, api)

	s.Handler = r
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

func (s *server) originMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && s.checkOrigin(r) {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, Authorization, Content-Type, X-Requested-With, Access-Control-Request-Headers, Access-Control-Request-Method, Swarm-Tag, Swarm-Pin, Swarm-Encrypt, Swarm-Index-Document, Swarm-Error-Document, Swarm-Collection, Swarm-Postage-Batch-Id, Gas-Price")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, DELETE")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}
		h.ServeHTTP(w, r)
	})
}
