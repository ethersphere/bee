// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"expvar"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/ethersphere/bee/pkg/auth"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging/httpaccess"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"resenje.org/web"
)

const (
	apiVersion = "v1" // Only one api version exists, this should be configurable with more.
	rootPath   = "/" + apiVersion
)

func (s *Service) newDebugRouter() *mux.Router {
	router := mux.NewRouter()

	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})

	router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	router.Handle("/node", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.nodeGetHandler),
	})

	router.Handle("/addresses", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.addressesHandler),
	})

	if s.transaction != nil {
		router.Handle("/transactions", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.transactionListHandler),
		})
		router.Handle("/transactions/{hash}", jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.transactionDetailHandler),
			"POST":   http.HandlerFunc(s.transactionResendHandler),
			"DELETE": http.HandlerFunc(s.transactionCancelHandler),
		})
	}

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
		web.FinalHandlerFunc(statusHandler),
	))

	return router
}

func (s *Service) setupRouting() {
	router := s.newDebugRouter()

	router.Handle("/readiness", web.ChainHandlers(
		httpaccess.SetAccessLogLevelHandler(0), // suppress access log messages
		web.FinalHandlerFunc(statusHandler),
	))

	// handle is a helper closure which simplifies the router setup.
	handle := func(path string, handler http.Handler) {
		if s.Restricted {
			handler = web.ChainHandlers(auth.PermissionCheckHandler(s.auth), web.FinalHandler(handler))
		}
		router.Handle(path, handler)
		router.Handle(rootPath+path, handler)
	}

	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})

	router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	if s.Restricted {
		router.Handle("/auth", jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.newTracingHandler("auth"),
				jsonhttp.NewMaxBodyBytesHandler(512),
				web.FinalHandlerFunc(s.authHandler),
			),
		})
		router.Handle("/refresh", jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.newTracingHandler("auth"),
				jsonhttp.NewMaxBodyBytesHandler(512),
				web.FinalHandlerFunc(s.refreshHandler),
			),
		})
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
			s.newTracingHandler("bytes-download"),
			web.FinalHandlerFunc(s.bytesGetHandler),
		),
	})

	handle("/chunks", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkWithSpanSize),
			web.FinalHandlerFunc(s.chunkUploadHandler),
		),
	})

	handle("/chunks/stream", web.ChainHandlers(
		s.newTracingHandler("chunks-stream-upload"),
		web.FinalHandlerFunc(s.chunkUploadStreamHandler),
	))

	handle("/chunks/{address}", jsonhttp.MethodHandler{
		"GET":    http.HandlerFunc(s.chunkGetHandler),
		"HEAD":   http.HandlerFunc(s.hasChunkHandler),
		"DELETE": http.HandlerFunc(s.removeChunk),
	})

	handle("/soc/{owner}/{id}", jsonhttp.MethodHandler{
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
	handle("/bzz/{address}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
	}))
	handle("/bzz/{address}/{path:.*}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.contentLengthMetricMiddleware(),
			s.newTracingHandler("bzz-download"),
			web.FinalHandlerFunc(s.bzzDownloadHandler),
		),
		"PATCH": web.ChainHandlers(
			s.newTracingHandler("bzz-patch"),
			web.FinalHandlerFunc(s.bzzPatchHandler),
		),
	})

	handle("/pss/send/{topic}/{targets}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkSize),
				web.FinalHandlerFunc(s.pssPostHandler),
			),
		})),
	)

	handle("/pss/subscribe/{topic}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandlerFunc(s.pssWsHandler),
	))

	handle("/tags", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.listTagsHandler),
			"POST": web.ChainHandlers(
				jsonhttp.NewMaxBodyBytesHandler(1024),
				web.FinalHandlerFunc(s.createTagHandler),
			),
		})),
	)
	handle("/tags/{id}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.getTagHandler),
			"DELETE": http.HandlerFunc(s.deleteTagHandler),
			"PATCH": web.ChainHandlers(
				jsonhttp.NewMaxBodyBytesHandler(1024),
				web.FinalHandlerFunc(s.doneSplitHandler),
			),
		})),
	)

	handle("/pins", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.listPinnedRootHashes),
		})),
	)
	handle("/pins/{reference}", web.ChainHandlers(
		s.gatewayModeForbidEndpointHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.getPinnedRootHash),
			"POST":   http.HandlerFunc(s.pinRootHash),
			"DELETE": http.HandlerFunc(s.unpinRootHash),
		})),
	)

	handle("/stewardship/{address}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.gatewayModeForbidEndpointHandler,
			web.FinalHandlerFunc(s.stewardshipGetHandler),
		),
		"PUT": web.ChainHandlers(
			s.gatewayModeForbidEndpointHandler,
			web.FinalHandlerFunc(s.stewardshipPutHandler),
		),
	})

	handle("/peers", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.peersHandler),
	})

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

	handle("/blocklist", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.blocklistedPeersHandler),
	})

	handle("/peers/{address}", jsonhttp.MethodHandler{
		"DELETE": http.HandlerFunc(s.peerDisconnectHandler),
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

	if s.swapEnabled {
		handle("/settlements", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.settlementsHandler),
		})

		handle("/settlements/{peer}", jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.peerSettlementsHandler),
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

	if s.chequebookEnabled {
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
	}

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

	handle("/batches", web.ChainHandlers(
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetAllStampsHandler),
		})),
	)

	handle("/wallet", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.walletHandler),
	})

	s.Handler = web.ChainHandlers(
		httpaccess.NewHTTPAccessLogHandler(s.logger, logrus.InfoLevel, s.tracer, "api access"),
		handlers.CompressHandler,
		// todo: add recovery handler
		s.responseCodeMetricsHandler,
		s.pageviewMetricsHandler,
		func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if o := r.Header.Get("Origin"); o != "" && s.checkOrigin(r) {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Origin", o)
					w.Header().Set("Access-Control-Allow-Headers", "User-Agent, Origin, Accept, Authorization, Content-Type, X-Requested-With, Decompressed-Content-Length, Access-Control-Request-Headers, Access-Control-Request-Method, Swarm-Tag, Swarm-Pin, Swarm-Encrypt, Swarm-Index-Document, Swarm-Error-Document, Swarm-Collection, Swarm-Postage-Batch-Id, Gas-Price")
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

func (s *Service) gatewayModeForbidEndpointHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.GatewayMode {
			s.logger.Tracef("gateway mode: forbidden %s", r.URL.String())
			jsonhttp.Forbidden(w, nil)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Service) gatewayModeForbidHeadersHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.GatewayMode {
			if strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true" {
				s.logger.Tracef("gateway mode: forbidden pinning %s", r.URL.String())
				jsonhttp.Forbidden(w, "pinning is disabled")
				return
			}
			if strings.ToLower(r.Header.Get(SwarmEncryptHeader)) == "true" {
				s.logger.Tracef("gateway mode: forbidden encryption %s", r.URL.String())
				jsonhttp.Forbidden(w, "encryption is disabled")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
