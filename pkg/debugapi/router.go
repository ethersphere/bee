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
func (s *Service) newBasicRouter() *mux.Router {
	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	router.Path("/metrics").Handler(web.ChainHandlers(
		httpaccess.SetAccessLogLevelHandler(0), // suppress access log messages
		web.FinalHandler(promhttp.InstrumentMetricHandler(
			s.metricsRegistry,
			promhttp.HandlerFor(s.metricsRegistry, promhttp.HandlerOpts{}),
		)),
	))

	router.Handle("/debug/pprof", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
	}))
	router.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	router.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	router.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	router.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	router.PathPrefix("/debug/pprof/").Handler(http.HandlerFunc(pprof.Index))

	router.Handle("/debug/vars", expvar.Handler())

	var handle = func(path string, handler http.Handler) {
		if s.restricted {
			handler = web.ChainHandlers(auth.PermissionCheckHandler(s.auth), web.FinalHandler(handler))
		}
		router.Handle(path, handler)
	}

	handle("/health", web.ChainHandlers(
		httpaccess.SetAccessLogLevelHandler(0), // suppress access log messages
		web.FinalHandlerFunc(statusHandler),
	))

	handle("/addresses", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.addressesHandler),
	})

	if s.transaction != nil {
		handle("/transactions", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.transactionListHandler),
		})
		handle("/transactions/{hash}", jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.transactionDetailHandler),
			"POST":   http.HandlerFunc(s.transactionResendHandler),
			"DELETE": http.HandlerFunc(s.transactionCancelHandler),
		})
	}

	return router
}

// newRouter construct the complete set of routes after all of the dependencies
// are injected and exposes /readiness endpoint to provide information that
// Debug API is fully active.
func (s *Service) newRouter() *mux.Router {
	router := s.newBasicRouter()

	var handle = func(path string, handler http.Handler) {
		if s.restricted {
			handler = web.ChainHandlers(auth.PermissionCheckHandler(s.auth), web.FinalHandler(handler))
		}
		router.Handle(path, handler)
	}

	handle("/readiness", web.ChainHandlers(
		httpaccess.SetAccessLogLevelHandler(0), // suppress access log messages
		web.FinalHandlerFunc(statusHandler),
	))

	handle("/pingpong/{peer-id}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.pingpongHandler),
	})

	handle("/reservestate", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.reserveStateHandler),
	})

	handle("/chainstate", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.chainStateHandler),
	})

	handle("/connect/{multi-address:.+}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.peerConnectHandler),
	})
	handle("/peers", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.peersHandler),
	})
	handle("/blocklist", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.blocklistedPeersHandler),
	})

	handle("/peers/{address}", jsonhttp.MethodHandler{
		"DELETE": http.HandlerFunc(s.peerDisconnectHandler),
	})
	handle("/chunks/{address}", jsonhttp.MethodHandler{
		"GET":    http.HandlerFunc(s.hasChunkHandler),
		"DELETE": http.HandlerFunc(s.removeChunk),
	})
	handle("/topology", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.topologyHandler),
	})
	handle("/welcome-message", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.getWelcomeMessageHandler),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(welcomeMessageMaxRequestSize),
			web.FinalHandlerFunc(s.setWelcomeMessageHandler),
		),
	})

	handle("/balances", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.compensatedBalancesHandler),
	})

	handle("/balances/{peer}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.compensatedPeerBalanceHandler),
	})

	handle("/consumed", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.balancesHandler),
	})

	handle("/consumed/{peer}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.peerBalanceHandler),
	})

	handle("/timesettlements", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.settlementsHandlerPseudosettle),
	})

	if s.chequebookEnabled {
		handle("/settlements", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.settlementsHandler),
		})
		handle("/settlements/{peer}", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.peerSettlementsHandler),
		})

		handle("/chequebook/balance", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookBalanceHandler),
		})

		handle("/chequebook/address", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookAddressHandler),
		})

		handle("/chequebook/deposit", jsonhttp.MethodHandler{
			"POST": http.HandlerFunc(s.chequebookDepositHandler),
		})

		handle("/chequebook/withdraw", jsonhttp.MethodHandler{
			"POST": http.HandlerFunc(s.chequebookWithdrawHandler),
		})

		handle("/chequebook/cheque/{peer}", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookLastPeerHandler),
		})

		handle("/chequebook/cheque", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookAllLastHandler),
		})

		handle("/chequebook/cashout/{peer}", jsonhttp.MethodHandler{
			"GET":  http.HandlerFunc(s.swapCashoutStatusHandler),
			"POST": http.HandlerFunc(s.swapCashoutHandler),
		})
	}

	handle("/tags/{id}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.getTagHandler),
	})

	handle("/stamps", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampsHandler),
		})),
	)

	handle("/stamps/{id}", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampHandler),
		})),
	)

	handle("/stamps/{id}/buckets", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampBucketsHandler),
		})),
	)

	handle("/stamps/{amount}/{depth}", web.ChainHandlers(
		s.postageAccessHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": http.HandlerFunc(s.postageCreateHandler),
		})),
	)

	handle("/stamps/topup/{id}/{amount}", web.ChainHandlers(
		s.postageAccessHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"PATCH": http.HandlerFunc(s.postageTopUpHandler),
		})),
	)

	handle("/stamps/dilute/{id}/{depth}", web.ChainHandlers(
		s.postageAccessHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"PATCH": http.HandlerFunc(s.postageDiluteHandler),
		})),
	)

	return router
}

// setRouter sets the base Debug API handler with common middlewares.
func (s *Service) setRouter(router http.Handler) {
	h := http.NewServeMux()
	h.Handle("/", web.ChainHandlers(
		httpaccess.NewHTTPAccessLogHandler(s.logger, logrus.InfoLevel, s.tracer, "debug api access"),
		handlers.CompressHandler,
		s.corsHandler,
		web.NoCacheHeadersHandler,
		web.FinalHandler(router),
	))

	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()

	s.handler = h
}
