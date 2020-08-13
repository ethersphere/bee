// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

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
		"POST": http.HandlerFunc(s.fileUploadHandler),
	})
	handle(router, "/files/{addr}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.fileDownloadHandler),
	})

	handle(router, "/dirs", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.dirUploadHandler),
	})

	handle(router, "/bytes", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.bytesUploadHandler),
	})
	handle(router, "/bytes/{address}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.bytesGetHandler),
	})

	handle(router, "/chunks/{addr}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.chunkGetHandler),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize),
			web.FinalHandlerFunc(s.chunkUploadHandler),
		),
	})

	handle(router, "/bzz/{address}/{path:.*}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.bzzDownloadHandler),
	})

	router.Handle("/tags", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.createTag),
	})

	router.Handle("/tags/{id}", jsonhttp.MethodHandler{
		"GET":    http.HandlerFunc(s.getTag),
		"DELETE": http.HandlerFunc(s.deleteTag),
		"PATCH":  http.HandlerFunc(s.doneSplit),
	})

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
