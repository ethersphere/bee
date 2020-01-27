// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"expvar"
	"net/http"
	"net/http/pprof"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	internalBaseRouter := http.NewServeMux()

	internalBaseRouter.Handle("/metrics", promhttp.InstrumentMetricHandler(
		s.metricsRegistry,
		promhttp.HandlerFor(s.metricsRegistry, promhttp.HandlerOpts{}),
	))

	internalRouter := mux.NewRouter()
	internalBaseRouter.Handle("/", web.ChainHandlers(
		handlers.CompressHandler,
		web.NoCacheHeadersHandler,
		web.FinalHandler(internalRouter),
	))
	internalRouter.Handle("/", http.NotFoundHandler())

	internalRouter.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	internalRouter.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	internalRouter.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	internalRouter.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	internalRouter.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	internalRouter.Handle("/debug/vars", expvar.Handler())

	internalRouter.HandleFunc("/health", s.statusHandler)
	internalRouter.HandleFunc("/readiness", s.statusHandler)

	internalRouter.HandleFunc("/connect/{multi-address:.+}", s.peerConnectHandler)

	s.Handler = internalBaseRouter
}
