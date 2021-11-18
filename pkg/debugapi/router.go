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

	r.Use(
		jsonhttp.RecovererMiddleware(s.logger),
		httpaccess.NewHTTPAccessLogHandler(s.logger, logrus.InfoLevel, s.tracer, "debug api access"),
		handlers.CompressHandler,
		s.corsHandler,
		web.NoCacheHeadersHandler,
	)

	r.NotFound(jsonhttp.NotFoundHandler)
	r.MethodNotAllowed(jsonhttp.MethodNotAllowedHandler)

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

	r.Group(func(r chi.Router) {

		if s.restricted {
			r.Use(auth.PermissionCheckHandler(s.auth))
		}

		r.Get("/peers", s.peersHandler)

		r.Post("/pingpong/{peer-id}", s.pingpongHandler)

		r.Get("/reservestate", s.reserveStateHandler)

		r.Get("/chainstate", s.chainStateHandler)

		r.Post("/connect/*", s.peerConnectHandler)

		r.Get("/blocklist", s.blocklistedPeersHandler)

		r.Delete("/peers/{address}", s.peerDisconnectHandler)

		r.Route("/chunks/{address}", func(r chi.Router) {
			r.Get("/", s.hasChunkHandler)
			r.Delete("/", s.removeChunk)
		})

		r.Get("/topology", s.topologyHandler)

		r.Route("/welcome-message", func(r chi.Router) {
			r.Get("/", s.getWelcomeMessageHandler)
			r.With(jsonhttp.NewMaxBodyBytesHandler(welcomeMessageMaxRequestSize)).Post("/", s.setWelcomeMessageHandler)
		})

		r.Get("/balances", s.compensatedBalancesHandler)

		r.Get("/balances/{peer}", s.compensatedPeerBalanceHandler)

		r.Get("/consumed", s.balancesHandler)

		r.Get("/consumed/{peer}", s.peerBalanceHandler)

		r.Get("/timesettlements", s.settlementsHandlerPseudosettle)

		if s.chequebookEnabled {

			r.Get("/settlements", s.settlementsHandler)
			r.Get("/settlements/{peer}", s.peerSettlementsHandler)

			r.Route("/chequebook", func(r chi.Router) {

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

		r.Get("/tags/{id}", s.getTagHandler)

		r.Get("/stamps", s.postageGetStampsHandler)

		r.Get("/stamps/{id}", s.postageGetStampHandler)

		r.Get("/stamps/{id}/buckets", s.postageGetStampBucketsHandler)

		r.With(s.postageAccessHandler).Post("/stamps/{amount}/{depth}", s.postageCreateHandler)
		r.With(s.postageAccessHandler).Patch("/stamps/topup/{id}/{amount}", s.postageTopUpHandler)
		r.With(s.postageAccessHandler).Patch("/stamps/dilute/{id}/{depth}", s.postageDiluteHandler)
	})

	return r
}

// setRouter sets the base Debug API handler with common middlewares.
func (s *Service) setRouter(router http.Handler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()

	s.handler = router
}
