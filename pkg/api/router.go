// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

func (s *Service) mountAPI() {
	subdomainRouter := s.router.Host("{subdomain:.*}.swarm.localhost").Subrouter()

	subdomainRouter.Handle("/{path:.*}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			web.FinalHandlerFunc(s.subdomainHandler),
		),
	})

	s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})

	s.router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	// handle is a helper closure which simplifies the router setup.
	handle := func(path string, handler http.Handler) {
		routeHandler := s.checkRouteAvailability(handler)
		s.router.Handle(path, routeHandler)
		s.router.Handle(rootPath+path, routeHandler)
	}

	handle("/bytes", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			s.contentLengthMetricMiddleware(),
			s.newTracingHandler("bytes-upload"),
			web.FinalHandlerFunc(s.bytesUploadHandler),
		),
	})

	handle("/bytes/{address}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.contentLengthMetricMiddleware(),
			s.downloadSpeedMetricMiddleware("bytes"),
			s.newTracingHandler("bytes-download"),
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bytesGetHandler),
		),
		"HEAD": web.ChainHandlers(
			s.newTracingHandler("bytes-head"),
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bytesHeadHandler),
		),
	})

	handle("/chunks", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.SocMaxChunkSize),
			web.FinalHandlerFunc(s.chunkUploadHandler),
		),
	})

	handle("/chunks/stream", web.ChainHandlers(
		s.newTracingHandler("chunks-stream-upload"),
		web.FinalHandlerFunc(s.chunkUploadStreamHandler),
	))

	handle("/chunks/{address}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.chunkGetHandler),
		),
		"HEAD": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.hasChunkHandler),
		),
	})

	handle("/envelope/{address}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.envelopePostHandler),
	})

	handle("/soc/{owner}/{id}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.socGetHandler),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize),
			web.FinalHandlerFunc(s.socUploadHandler),
		),
	})

	handle("/feeds/{owner}/{topic}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.feedGetHandler),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize),
			web.FinalHandlerFunc(s.feedPostHandler),
		),
	})

	handle("/bzz", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			s.contentLengthMetricMiddleware(),
			s.newTracingHandler("bzz-upload"),
			web.FinalHandlerFunc(s.bzzUploadHandler),
		),
	})

	handle("/grantee", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.actCreateGranteesHandler),
	})

	handle("/grantee/{address}", jsonhttp.MethodHandler{
		"GET":   http.HandlerFunc(s.actListGranteesHandler),
		"PATCH": http.HandlerFunc(s.actGrantRevokeHandler),
	})

	handle("/bzz/{address}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
	}))

	handle("/bzz/{address}/{path:.*}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.contentLengthMetricMiddleware(),
			s.newTracingHandler("bzz-download"),
			s.actDecryptionHandler(),
			s.downloadSpeedMetricMiddleware("bzz"),
			web.FinalHandlerFunc(s.bzzDownloadHandler),
		),
		"HEAD": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bzzHeadHandler),
		),
	})

	handle("/pss/send/{topic}/{targets}", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkSize),
			web.FinalHandlerFunc(s.pssPostHandler),
		),
	})

	handle("/pss/subscribe/{topic}", http.HandlerFunc(s.pssWsHandler))

	handle("/gsoc/subscribe/{address}", web.ChainHandlers(
		web.FinalHandlerFunc(s.gsocWsHandler),
	))

	handle("/tags", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.listTagsHandler),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(1024),
			web.FinalHandlerFunc(s.createTagHandler),
		),
	})

	handle("/tags/{id}", jsonhttp.MethodHandler{
		"GET":    http.HandlerFunc(s.getTagHandler),
		"DELETE": http.HandlerFunc(s.deleteTagHandler),
		"PATCH": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(1024),
			web.FinalHandlerFunc(s.doneSplitHandler),
		),
	})

	handle("/pins", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.listPinnedRootHashes),
	})

	handle("/pins/check", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.pinIntegrityHandler),
	})

	handle("/pins/{reference}", jsonhttp.MethodHandler{
		"GET":    http.HandlerFunc(s.getPinnedRootHash),
		"POST":   http.HandlerFunc(s.pinRootHash),
		"DELETE": http.HandlerFunc(s.unpinRootHash),
	},
	)

	handle("/stewardship/{address}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.stewardshipGetHandler),
		"PUT": http.HandlerFunc(s.stewardshipPutHandler),
	})
}
