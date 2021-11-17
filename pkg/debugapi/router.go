// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"expvar"
	"net/http"
	"net/http/pprof"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/handlers"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"resenje.org/web"

	"github.com/ethersphere/bee/pkg/auth"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging/httpaccess"
)

// newBasicRouter constructs only the routes that do not depend on the injected dependencies:
// - /health
// - pprof
// - vars
// - metrics
// - /addresses
func (s *Service) newBasicRouter() chi.Router {

	r := chi.NewRouter()

	r.NotFound(jsonhttp.NotFoundHandler)
	r.MethodNotAllowed(jsonhttp.MethodNotAllowedHandler)

	r.Use(
		httpaccess.NewHTTPAccessLogHandler(s.logger, logrus.InfoLevel, s.tracer, "debug api access"),
		handlers.CompressHandler,
		s.corsHandler,
		web.NoCacheHeadersHandler,
	)

	r.With(httpaccess.SetAccessLogLevelHandler(0)).Handle("/metrics", promhttp.InstrumentMetricHandler(
		s.metricsRegistry,
		promhttp.HandlerFor(s.metricsRegistry, promhttp.HandlerOpts{}),
	))

	r.Route("/debug", func(r chi.Router) {
		r.HandleFunc("/pprof", func(w http.ResponseWriter, r *http.Request) {
			u := r.URL
			u.Path += "/"
			http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
		})
		r.Handle("/vars", expvar.Handler())
		r.HandleFunc("/pprof/cmdline", pprof.Cmdline)
		r.HandleFunc("/pprof/profile", pprof.Profile)
		r.HandleFunc("/pprof/symbol", pprof.Symbol)
		r.HandleFunc("/pprof/trace", pprof.Trace)
		r.HandleFunc("/pprof/", pprof.Index)
	})

	r.With(httpaccess.SetAccessLogLevelHandler(0)).HandleFunc("/health", statusHandler)

	r.Group(func(r chi.Router) {

		if s.restricted {
			r.Use(auth.PermissionCheckHandler(s.auth))
		}

		r.Get("/addresses", s.addressesHandler)

		if s.transaction != nil {
			r.Route("/transactions", func(r chi.Router) {
				r.Get("/", s.transactionListHandler)
				r.Get("/{hash}", s.transactionDetailHandler)
				r.Post("/{hash}", s.transactionResendHandler)
				r.Delete("/{hash}", s.transactionCancelHandler)
			})
		}
	})

	return r
}

// newRouter construct the complete set of routes after all of the dependencies
// are injected and exposes /readiness endpoint to provide information that
// Debug API is fully active.
func (s *Service) newRouter() chi.Router {

	r := s.newBasicRouter()

	r.With(httpaccess.SetAccessLogLevelHandler(0)).HandleFunc("/readiness", statusHandler)

	r.Group(func(api chi.Router) {

		if s.restricted {
			api.Use(auth.PermissionCheckHandler(s.auth))
		}

		api.Get("/peers", s.peersHandler)

		api.Post("/pingpong/{peer-id}", s.pingpongHandler)

		api.Get("/reservestate", s.reserveStateHandler)

		api.Get("/chainstate", s.chainStateHandler)

		api.Post("/connect/*", s.peerConnectHandler)

		api.Get("/blocklist", s.blocklistedPeersHandler)

		api.Delete("/peers/{address}", s.peerDisconnectHandler)

		api.Route("/chunks/{address}", func(r chi.Router) {
			r.Get("/", s.hasChunkHandler)
			r.Delete("/", s.removeChunk)
		})

		api.Get("/topology", s.topologyHandler)

		api.Route("/welcome-message", func(r chi.Router) {
			r.Get("/", s.getWelcomeMessageHandler)
			r.With(jsonhttp.NewMaxBodyBytesHandler(welcomeMessageMaxRequestSize)).Post("/", s.setWelcomeMessageHandler)
		})

		api.Get("/balances", s.compensatedBalancesHandler)

		api.Get("/balances/{peer}", s.compensatedPeerBalanceHandler)

		api.Get("/consumed", s.balancesHandler)

		api.Get("/consumed/{peer}", s.peerBalanceHandler)

		api.Get("/timesettlements", s.settlementsHandlerPseudosettle)

		if s.chequebookEnabled {

			api.Get("/settlements", s.settlementsHandler)
			api.Get("/settlements/{peer}", s.peerSettlementsHandler)

			api.Route("/chequebook", func(r chi.Router) {

				r.Get("/balance", s.chequebookBalanceHandler)

				r.Get("/address", s.chequebookAddressHandler)

				r.Post("/deposit", s.chequebookDepositHandler)

				r.Post("/withdraw", s.chequebookWithdrawHandler)

				r.Get("/cheque/{peer}", s.chequebookLastPeerHandler)

				r.Get("/cheque", s.chequebookAllLastHandler)

				r.Route("/cashout/{peer}", func(r chi.Router) {
					r.Get("/", s.swapCashoutStatusHandler)
					r.Post("/", s.swapCashoutHandler)
				})
			})
		}

		api.Get("/tags/{id}", s.getTagHandler)

		api.Get("/stamps", s.postageGetStampsHandler)

		api.Get("/stamps/{id}", s.postageGetStampHandler)

		api.Get("/stamps/{id}/buckets", s.postageGetStampBucketsHandler)

		api.With(s.postageAccessHandler).Post("/stamps/{amount}/{depth}", s.postageCreateHandler)
		api.With(s.postageAccessHandler).Patch("/stamps/topup/{id}/{amount}", s.postageTopUpHandler)
		api.With(s.postageAccessHandler).Patch("/stamps/dilute/{id}/{depth}", s.postageDiluteHandler)
	})

	return r
}

// setRouter sets the base Debug API handler with common middlewares.
func (s *Service) setRouter(router http.Handler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()

	s.handler = router
}
