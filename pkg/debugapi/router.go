// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"expvar"
	"net/http"
	"net/http/pprof"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	baseRouter := http.NewServeMux()

	baseRouter.Handle("/metrics", promhttp.InstrumentMetricHandler(
		s.metricsRegistry,
		promhttp.HandlerFor(s.metricsRegistry, promhttp.HandlerOpts{}),
	))

	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	router.Handle("/debug/pprof", http.HandlerFunc(pprof.Index))
	router.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	router.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	router.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	router.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	router.PathPrefix("/debug/pprof/").Handler(http.HandlerFunc(pprof.Index))

	router.Handle("/debug/vars", expvar.Handler())

	router.HandleFunc("/health", s.statusHandler)
	router.HandleFunc("/readiness", s.statusHandler)

	router.Handle("/addresses", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.addressesHandler),
	})
	router.Handle("/connect/{multi-address:.+}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.peerConnectHandler),
	})
	router.Handle("/peers", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.peersHandler),
	})
	router.Handle("/peers/{address}", jsonhttp.MethodHandler{
		"DELETE": http.HandlerFunc(s.peerDisconnectHandler),
	})
	router.Handle("/chunks/{address}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.hasChunkHandler),
	})
	router.Handle("/chunks-pin/{address}", jsonhttp.MethodHandler{
		"POST":   http.HandlerFunc(s.pinChunk),
		"DELETE": http.HandlerFunc(s.unpinChunk),
		"GET":    http.HandlerFunc(s.listPinnedChunks),
	})
	router.Handle("/chunks-pin", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.listPinnedChunks),
	})
	router.Handle("/topology", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.topologyJsonHandler),
	})

	baseRouter.Handle("/", web.ChainHandlers(
		logging.NewHTTPAccessLogHandler(s.Logger, logrus.InfoLevel, "debug api access"),
		handlers.CompressHandler,
		// todo: add recovery handler
		web.NoCacheHeadersHandler,
		web.FinalHandler(router),
	))

	s.Handler = baseRouter
}
