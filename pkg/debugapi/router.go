// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"expvar"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"resenje.org/web"

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

	router.Handle("/health", web.ChainHandlers(
		httpaccess.SetAccessLogLevelHandler(0), // suppress access log messages
		s.permissionCheckHandler(),
		web.FinalHandlerFunc(statusHandler),
	))

	router.Handle("/addresses", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.addressesHandler)),
	})

	if s.transaction != nil {
		router.Handle("/transactions", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.transactionListHandler)),
		})
		router.Handle("/transactions/{hash}", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.transactionDetailHandler)),
			"POST": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.transactionResendHandler)),
			"DELETE": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.transactionCancelHandler)),
		})
	}

	return router
}

// newRouter construct the complete set of routes after all of the dependencies
// are injected and exposes /readiness endpoint to provide information that
// Debug API is fully active.
func (s *Service) newRouter() *mux.Router {
	router := s.newBasicRouter()

	router.Handle("/readiness", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.SetAccessLogLevelHandler(0), // suppress access log messages
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(statusHandler)),
	})

	router.Handle("/pingpong/{peer-id}", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.pingpongHandler)),
	})

	router.Handle("/reservestate", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.reserveStateHandler)),
	})

	router.Handle("/chainstate", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.chainStateHandler)),
	})

	router.Handle("/connect/{multi-address:.+}", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.peerConnectHandler)),
	})
	router.Handle("/peers", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.peersHandler)),
	})
	router.Handle("/blocklist", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.blocklistedPeersHandler)),
	})

	router.Handle("/peers/{address}", jsonhttp.MethodHandler{
		"DELETE": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.peerDisconnectHandler)),
	})
	router.Handle("/chunks/{address}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.hasChunkHandler)),
		"DELETE": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.removeChunk)),
	})
	router.Handle("/topology", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.topologyHandler)),
	})
	router.Handle("/welcome-message", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.getWelcomeMessageHandler)),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(welcomeMessageMaxRequestSize),
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.setWelcomeMessageHandler),
		),
	})

	router.Handle("/balances", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.compensatedBalancesHandler)),
	})

	router.Handle("/balances/{peer}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.compensatedPeerBalanceHandler)),
	})

	router.Handle("/consumed", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.balancesHandler),
	})

	router.Handle("/consumed/{peer}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.peerBalanceHandler),
	})

	router.Handle("/timesettlements", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.settlementsHandlerPseudosettle)),
	})

	if s.chequebookEnabled {
		router.Handle("/settlements", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.settlementsHandler)),
		})
		router.Handle("/settlements/{peer}", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.peerSettlementsHandler)),
		})

		router.Handle("/chequebook/balance", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.chequebookBalanceHandler)),
		})

		router.Handle("/chequebook/address", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.chequebookAddressHandler)),
		})

		router.Handle("/chequebook/deposit", jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.chequebookDepositHandler)),
		})

		router.Handle("/chequebook/withdraw", jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.chequebookWithdrawHandler)),
		})

		router.Handle("/chequebook/cheque/{peer}", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.chequebookLastPeerHandler)),
		})

		router.Handle("/chequebook/cheque", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.chequebookAllLastHandler)),
		})

		router.Handle("/chequebook/cashout/{peer}", jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.swapCashoutStatusHandler)),
			"POST": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.swapCashoutHandler)),
		})
	}

	router.Handle("/tags/{id}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.permissionCheckHandler(),
			web.FinalHandlerFunc(s.getTagHandler)),
	})

	router.Handle("/stamps", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.postageGetStampsHandler)),
		})),
	)

	router.Handle("/stamps/{id}", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.postageGetStampHandler)),
		})),
	)

	router.Handle("/stamps/{id}/buckets", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.postageGetStampBucketsHandler)),
		})),
	)

	router.Handle("/stamps/{amount}/{depth}", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.permissionCheckHandler(),
				web.FinalHandlerFunc(s.postageCreateHandler)),
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

func (s *Service) permissionCheckHandler() func(h http.Handler) http.Handler {
	if !s.restricted {
		return func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				h.ServeHTTP(w, r)
			})
		}
	}

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqToken := r.Header.Get("Authorization")
			if !strings.HasPrefix(reqToken, "Bearer ") {
				jsonhttp.Forbidden(w, "Missing bearer token")
				return
			}

			keys := strings.Split(reqToken, "Bearer ")

			if len(keys) != 2 || strings.Trim(keys[1], " ") == "" {
				jsonhttp.Forbidden(w, "Missing security token")
				return
			}

			apiKey := keys[1]

			allowed, err := s.auth.Enforce(apiKey, r.URL.Path, r.Method)
			if err != nil {
				jsonhttp.InternalServerError(w, "Validate security token")
				return
			}

			if !allowed {
				jsonhttp.Forbidden(w, "Provided security token does not grant access to the resource")
				return
			}

			h.ServeHTTP(w, r)
		})
	}
}
