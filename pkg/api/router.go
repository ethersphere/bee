// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	apiVersion := "v1" // only one api version exists, this should be configurable with more

	handle := func(router *mux.Router, path string, handler http.Handler) {
		router.Handle(path, handler)
		router.Handle("/"+apiVersion+path, handler)
	}

	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})

	router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	handle(router, "/files", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			s.newTracingHandler("files-upload"),
			web.FinalHandlerFunc(s.fileUploadHandler),
		),
	})
	handle(router, "/files/{addr}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.newTracingHandler("files-download"),
			web.FinalHandlerFunc(s.fileDownloadHandler),
		),
	})

	handle(router, "/dirs", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			s.newTracingHandler("dirs-upload"),
			web.FinalHandlerFunc(s.dirUploadHandler),
		),
	})

	handle(router, "/bytes", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			s.newTracingHandler("bytes-upload"),
			web.FinalHandlerFunc(s.bytesUploadHandler),
		),
	})
	handle(router, "/bytes/{address}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.newTracingHandler("bytes-download"),
			web.FinalHandlerFunc(s.bytesGetHandler),
		),
	})

	handle(router, "/chunks/{addr}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.chunkGetHandler),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize),
			web.FinalHandlerFunc(s.chunkUploadHandler),
		),
	})

	handle(router, "/bzz/{address}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
	}))
	handle(router, "/bzz/{address}/{path:.*}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.newTracingHandler("bzz-download"),
			web.FinalHandlerFunc(s.bzzDownloadHandler),
		),
	})

	handle(router, "/pss/send/{topic}/{targets}", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkSize),
			web.FinalHandlerFunc(s.pssPostHandler),
		),
	})

	handle(router, "/pss/subscribe/{topic}", http.HandlerFunc(s.pssWsHandler))

	handle(router, "/tags", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				jsonhttp.NewMaxBodyBytesHandler(1024),
				web.FinalHandlerFunc(s.createTag),
			),
		})),
	)
	handle(router, "/tags/{id}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.getTag),
			"DELETE": http.HandlerFunc(s.deleteTag),
			"PATCH": web.ChainHandlers(
				jsonhttp.NewMaxBodyBytesHandler(1024),
				web.FinalHandlerFunc(s.doneSplit),
			),
		})),
	)

	handle(router, "/pinning/chunks/{address}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.getPinnedChunk),
			"POST":   http.HandlerFunc(s.pinChunk),
			"DELETE": http.HandlerFunc(s.unpinChunk),
		})),
	)
	handle(router, "/pinning/chunks", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.listPinnedChunks),
		})),
	)

	handle(router, "/pinning/bytes/{address}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST":   http.HandlerFunc(s.pinBytesUploaded),
			"DELETE": http.HandlerFunc(s.pinBytesRemovePinned),
		})),
	)

	handle(router, "/pinning/files/{address}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST":   http.HandlerFunc(s.pinFilesUploaded),
			"DELETE": http.HandlerFunc(s.pinFilesRemovePinned),
		})),
	)

	handle(router, "/pinning/bzz/{address}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST":   http.HandlerFunc(s.pinBzzUploaded),
			"DELETE": http.HandlerFunc(s.pinBzzRemovePinned),
		})),
	)

	s.Handler = web.ChainHandlers(
		logging.NewHTTPAccessLogHandler(s.Logger, logrus.InfoLevel, "api access"),
		handlers.CompressHandler,
		// todo: add recovery handler
		s.pageviewMetricsHandler,
		func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if o := r.Header.Get("Origin"); o != "" && (s.CORSAllowedOrigins == nil || containsOrigin(o, s.CORSAllowedOrigins)) {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Origin", o)
					w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, Authorization, Content-Type, X-Requested-With, Access-Control-Request-Headers, Access-Control-Request-Method")
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

func containsOrigin(s string, l []string) (ok bool) {
	for _, e := range l {
		if e == s || e == "*" {
			return true
		}
	}
	return false
}

func (s *server) gatewayModeForbidEndpointHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.GatewayMode {
			s.Logger.Tracef("gateway mode: forbidden %s", r.URL.String())
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
				s.Logger.Tracef("gateway mode: forbidden pinning %s", r.URL.String())
				jsonhttp.Forbidden(w, "pinning is disabled")
				return
			}
			if strings.ToLower(r.Header.Get(SwarmEncryptHeader)) == "true" {
				s.Logger.Tracef("gateway mode: forbidden encryption %s", r.URL.String())
				jsonhttp.Forbidden(w, "encryption is disabled")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
