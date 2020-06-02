// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})

	router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	router.Handle("/pingpong/{peer-id}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.pingpongHandler),
	})

	router.Handle("/bzz", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.bzzUploadHandler),
	})

	router.Handle("/bzz/{address}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.bzzGetHandler),
	})

	router.Handle("/bzz-chunk/{addr}", jsonhttp.MethodHandler{
		"GET":  http.HandlerFunc(s.chunkGetHandler),
		"POST": http.HandlerFunc(s.chunkUploadHandler),
	})

	router.Handle("/bzz-tag/name/{name}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.CreateTag),
	})

	router.Handle("/bzz-tag/addr/{addr}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.getTagInfoUsingAddress),
	})

	router.Handle("/bzz-tag/uuid/{uuid}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.getTagInfoUsingUUid),
	})

	s.Handler = web.ChainHandlers(
		logging.NewHTTPAccessLogHandler(s.Logger, logrus.InfoLevel, "api access"),
		handlers.CompressHandler,
		// todo: add recovery handler
		s.pageviewMetricsHandler,
		web.FinalHandler(router),
	)
}
