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

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/felixge/fgprof"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"resenje.org/web"
)

const (
	apiVersion = "v1" // Only one api version exists, this should be configurable with more.
	rootPath   = "/" + apiVersion
)

func (s *Service) Mount() {
	if s == nil {
		return
	}

	router := mux.NewRouter()

	router.NotFoundHandler = http.HandlerFunc(jsonhttp.NotFoundHandler)

	s.router = router

	s.mountTechnicalDebug()
	s.mountBusinessDebug()
	s.mountAPI()

	s.Handler = web.ChainHandlers(
		httpaccess.NewHTTPAccessLogHandler(s.logger, s.tracer, "api access"),
		handlers.CompressHandler,
		s.corsHandler,
		web.NoCacheHeadersHandler,
		web.FinalHandler(router),
	)
}

// EnableFullAPI will enable all available endpoints, because some endpoints are not available during syncing.
func (s *Service) EnableFullAPI() {
	if s == nil {
		return
	}

	s.fullAPIEnabled = true

	compressHandler := func(h http.Handler) http.Handler {
		downloadEndpoints := []string{
			"/bzz",
			"/bytes",
			"/chunks",
			"/feeds",
			"/soc",
			rootPath + "/bzz",
			rootPath + "/bytes",
			rootPath + "/chunks",
			rootPath + "/feeds",
			rootPath + "/soc",
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip compression for GET requests on download endpoints.
			// This is done in order to preserve Content-Length header in response,
			// because CompressHandler is always removing it.
			if r.Method == http.MethodGet {
				for _, endpoint := range downloadEndpoints {
					if strings.HasPrefix(r.URL.Path, endpoint) {
						h.ServeHTTP(w, r)
						return
					}
				}
			}

			if r.Method == http.MethodHead {
				h.ServeHTTP(w, r)
				return
			}

			handlers.CompressHandler(h).ServeHTTP(w, r)
		})
	}

	s.Handler = web.ChainHandlers(
		httpaccess.NewHTTPAccessLogHandler(s.logger, s.tracer, "api access"),
		compressHandler,
		s.responseCodeMetricsHandler,
		s.pageviewMetricsHandler,
		s.corsHandler,
		web.FinalHandler(s.router),
	)
}

func (s *Service) mountTechnicalDebug() {
	s.router.Handle("/node", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.nodeGetHandler),
	})

	s.router.Handle("/addresses", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.addressesHandler),
	})

	s.router.Handle("/chainstate", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.chainStateHandler),
	})

	s.router.Handle("/debugstore", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			web.FinalHandlerFunc(s.debugStorage),
		),
	})

	s.router.Path("/metrics").Handler(web.ChainHandlers(
		httpaccess.NewHTTPAccessSuppressLogHandler(),
		web.FinalHandler(promhttp.InstrumentMetricHandler(
			s.metricsRegistry,
			promhttp.HandlerFor(s.metricsRegistry, promhttp.HandlerOpts{}),
		)),
	))

	s.router.Handle("/debug/pprof", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
	}))

	s.router.Handle("/debug/fgprof", fgprof.Handler())
	s.router.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	s.router.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	s.router.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	s.router.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	s.router.PathPrefix("/debug/pprof/").Handler(http.HandlerFunc(pprof.Index))
	s.router.Handle("/debug/vars", expvar.Handler())

	s.router.Handle("/loggers", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			web.FinalHandlerFunc(s.loggerGetHandler),
		),
	})

	s.router.Handle("/loggers/{exp}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			web.FinalHandlerFunc(s.loggerGetHandler),
		),
	})

	s.router.Handle("/loggers/{exp}/{verbosity}", jsonhttp.MethodHandler{
		"PUT": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			web.FinalHandlerFunc(s.loggerSetVerbosityHandler),
		),
	})

	s.router.Handle("/readiness", web.ChainHandlers(
		httpaccess.NewHTTPAccessSuppressLogHandler(),
		web.FinalHandlerFunc(s.readinessHandler),
	))

	s.router.Handle("/health", web.ChainHandlers(
		httpaccess.NewHTTPAccessSuppressLogHandler(),
		web.FinalHandlerFunc(s.healthHandler),
	))
}

func (s *Service) checkRouteAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.fullAPIEnabled {
			jsonhttp.ServiceUnavailable(w, "Node is syncing. This endpoint is unavailable. Try again later.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Service) checkSwapAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.swapEnabled {
			jsonhttp.NotImplemented(w, "Swap is disabled. This endpoint is unavailable.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Service) checkChequebookAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.chequebookEnabled {
			jsonhttp.NotImplemented(w, "Chequebook is disabled. This endpoint is unavailable.")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (s *Service) mountAPI() {
	subdomainRouter := s.router.Host("{subdomain:.*}.swarm.localhost").Subrouter()

	subdomainRouter.Handle("/{path:.*}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			web.FinalHandlerFunc(s.subdomainHandler),
		),
	})

	s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Ethereum Swarm Bee")
	})

	s.router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	// handle is a helper closure which simplifies the router setup.
	handle := func(path string, handler http.Handler) {
		routeHandler := s.checkRouteAvailability(handler)
		s.router.Handle(path, routeHandler)
		s.router.Handle(rootPath+path, routeHandler)
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
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bytesGetHandler),
		),
		"HEAD": web.ChainHandlers(
			s.newTracingHandler("bytes-head"),
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bytesHeadHandler),
		),
	})

	handle("/chunks", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.SocMaxChunkSize),
			web.FinalHandlerFunc(s.chunkUploadHandler),
		),
	})

	handle("/chunks/stream", web.ChainHandlers(
		s.newTracingHandler("chunks-stream-upload"),
		web.FinalHandlerFunc(s.chunkUploadStreamHandler),
	))

	handle("/chunks/{address}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.chunkGetHandler),
		),
		"HEAD": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.hasChunkHandler),
		),
	})

	handle("/envelope/{address}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.envelopePostHandler),
	})

	handle("/soc/{owner}/{id}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.socGetHandler),
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

	handle("/grantee", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.actCreateGranteesHandler),
	})

	handle("/grantee/{address}", jsonhttp.MethodHandler{
		"GET":   http.HandlerFunc(s.actListGranteesHandler),
		"PATCH": http.HandlerFunc(s.actGrantRevokeHandler),
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
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bzzDownloadHandler),
		),
		"HEAD": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bzzHeadHandler),
		),
	})

	handle("/pss/send/{topic}/{targets}", jsonhttp.MethodHandler{
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(swarm.ChunkSize),
			web.FinalHandlerFunc(s.pssPostHandler),
		),
	})

	handle("/pss/subscribe/{topic}", http.HandlerFunc(s.pssWsHandler))

	handle("/gsoc/subscribe/{address}", web.ChainHandlers(
		web.FinalHandlerFunc(s.gsocWsHandler),
	))

	handle("/tags", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.listTagsHandler),
		"POST": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(1024),
			web.FinalHandlerFunc(s.createTagHandler),
		),
	})

	handle("/tags/{id}", jsonhttp.MethodHandler{
		"GET":    http.HandlerFunc(s.getTagHandler),
		"DELETE": http.HandlerFunc(s.deleteTagHandler),
		"PATCH": web.ChainHandlers(
			jsonhttp.NewMaxBodyBytesHandler(1024),
			web.FinalHandlerFunc(s.doneSplitHandler),
		),
	})

	handle("/pins", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.listPinnedRootHashes),
	})

	handle("/pins/check", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.pinIntegrityHandler),
	})

	handle("/pins/{reference}", jsonhttp.MethodHandler{
		"GET":    http.HandlerFunc(s.getPinnedRootHash),
		"POST":   http.HandlerFunc(s.pinRootHash),
		"DELETE": http.HandlerFunc(s.unpinRootHash),
	},
	)

	handle("/stewardship/{address}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.stewardshipGetHandler),
		"PUT": http.HandlerFunc(s.stewardshipPutHandler),
	})
}

func (s *Service) mountBusinessDebug() {
	handle := func(path string, handler http.Handler) {
		routeHandler := s.checkRouteAvailability(handler)
		s.router.Handle(path, routeHandler)
		s.router.Handle(rootPath+path, routeHandler)
	}

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

	handle("/peers", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.peersHandler),
	})

	handle("/pingpong/{address}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.pingpongHandler),
	})

	handle("/reservestate", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.reserveStateHandler),
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

	handle("/settlements", web.ChainHandlers(
		s.checkSwapAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.settlementsHandler),
		}),
	))

	handle("/settlements/{peer}", web.ChainHandlers(
		s.checkSwapAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.peerSettlementsHandler),
		}),
	))

	handle("/chequebook/cheque/{peer}", web.ChainHandlers(
		s.checkSwapAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookLastPeerHandler),
		}),
	))

	handle("/chequebook/cheque", web.ChainHandlers(
		s.checkSwapAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookAllLastHandler),
		}),
	))

	handle("/chequebook/cashout/{peer}", web.ChainHandlers(
		s.checkSwapAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.swapCashoutStatusHandler),
			"POST": web.ChainHandlers(
				s.gasConfigMiddleware("swap cashout"),
				web.FinalHandlerFunc(s.swapCashoutHandler),
			),
		}),
	))

	handle("/chequebook/balance", web.ChainHandlers(
		s.checkChequebookAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookBalanceHandler),
		}),
	))

	handle("/chequebook/address", web.ChainHandlers(
		s.checkChequebookAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookAddressHandler),
		}),
	))

	handle("/chequebook/deposit", web.ChainHandlers(
		s.checkChequebookAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.gasConfigMiddleware("chequebook deposit"),
				web.FinalHandlerFunc(s.chequebookDepositHandler),
			),
		}),
	))

	handle("/chequebook/withdraw", web.ChainHandlers(
		s.checkChequebookAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.gasConfigMiddleware("chequebook withdraw"),
				web.FinalHandlerFunc(s.chequebookWithdrawHandler),
			),
		}),
	))

	handle("/wallet", web.ChainHandlers(
		s.checkChequebookAvailability,
		s.checkSwapAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.walletHandler),
		}),
	))

	handle("/wallet/withdraw/{coin}", web.ChainHandlers(
		s.checkChequebookAvailability,
		s.checkSwapAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": web.ChainHandlers(
				s.gasConfigMiddleware("wallet withdraw"),
				web.FinalHandlerFunc(s.walletWithdrawHandler),
			),
		}),
	))

	handle("/stamps", web.ChainHandlers(
		s.postageSyncStatusCheckHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampsHandler),
		})),
	)

	handle("/stamps/{batch_id}", web.ChainHandlers(
		s.postageSyncStatusCheckHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampHandler),
		})),
	)

	handle("/stamps/{batch_id}/buckets", web.ChainHandlers(
		s.postageSyncStatusCheckHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampBucketsHandler),
		})),
	)

	handle("/stamps/{amount}/{depth}", web.ChainHandlers(
		s.postageAccessHandler,
		s.postageSyncStatusCheckHandler,
		s.gasConfigMiddleware("create batch"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": http.HandlerFunc(s.postageCreateHandler),
		})),
	)

	handle("/stamps/topup/{batch_id}/{amount}", web.ChainHandlers(
		s.postageAccessHandler,
		s.postageSyncStatusCheckHandler,
		s.gasConfigMiddleware("topup batch"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"PATCH": http.HandlerFunc(s.postageTopUpHandler),
		})),
	)

	handle("/stamps/dilute/{batch_id}/{depth}", web.ChainHandlers(
		s.postageAccessHandler,
		s.postageSyncStatusCheckHandler,
		s.gasConfigMiddleware("dilute batch"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"PATCH": http.HandlerFunc(s.postageDiluteHandler),
		})),
	)

	handle("/batches", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.postageGetAllBatchesHandler),
	})

	handle("/accounting", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.accountingInfoHandler),
	})

	handle("/stake/withdrawable", web.ChainHandlers(
		s.stakingAccessHandler,
		s.gasConfigMiddleware("get or withdraw withdrawable stake"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.getWithdrawableStakeHandler),
			"DELETE": http.HandlerFunc(s.withdrawStakeHandler),
		})),
	)

	handle("/stake/{amount}", web.ChainHandlers(
		s.stakingAccessHandler,
		s.gasConfigMiddleware("deposit stake"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": http.HandlerFunc(s.stakingDepositHandler),
		}),
	))

	handle("/stake", web.ChainHandlers(
		s.stakingAccessHandler,
		s.gasConfigMiddleware("get or migrate stake"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET":    http.HandlerFunc(s.getPotentialStake),
			"DELETE": http.HandlerFunc(s.migrateStakeHandler),
		})),
	)

	handle("/redistributionstate", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.redistributionStatusHandler),
	})

	handle("/status", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			web.FinalHandlerFunc(s.statusGetHandler),
		),
	})

	handle("/status/peers", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			s.statusAccessHandler,
			web.FinalHandlerFunc(s.statusGetPeersHandler),
		),
	})

	handle("/status/neighborhoods", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			s.statusAccessHandler,
			web.FinalHandlerFunc(s.statusGetNeighborhoods),
		),
	})

	handle("/rchash/{depth}/{anchor1}/{anchor2}", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.rchash),
	})
}
