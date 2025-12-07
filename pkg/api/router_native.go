//go:build !js

package api

import (
	"expvar"
	"net/http"
	"net/http/pprof"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/felixge/fgprof"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"resenje.org/web"
)

func (s *Service) checkStorageIncentivesAvailability(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.redistributionAgent == nil {
			jsonhttp.Forbidden(w, "Storage incentives are disabled. This endpoint is unavailable.")
			return
		}
		handler.ServeHTTP(w, r)
	})
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
		s.checkChainAvailability,
		s.postageSyncStatusCheckHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampsHandler),
		})),
	)

	handle("/stamps/{batch_id}", web.ChainHandlers(
		s.checkChainAvailability,
		s.postageSyncStatusCheckHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampHandler),
		})),
	)

	handle("/stamps/{batch_id}/buckets", web.ChainHandlers(
		s.checkChainAvailability,
		s.postageSyncStatusCheckHandler,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.postageGetStampBucketsHandler),
		})),
	)

	handle("/stamps/{amount}/{depth}", web.ChainHandlers(
		s.checkChainAvailability,
		s.postageAccessHandler,
		s.postageSyncStatusCheckHandler,
		s.gasConfigMiddleware("create batch"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"POST": http.HandlerFunc(s.postageCreateHandler),
		})),
	)

	handle("/stamps/topup/{batch_id}/{amount}", web.ChainHandlers(
		s.checkChainAvailability,
		s.postageAccessHandler,
		s.postageSyncStatusCheckHandler,
		s.gasConfigMiddleware("topup batch"),
		web.FinalHandler(jsonhttp.MethodHandler{
			"PATCH": http.HandlerFunc(s.postageTopUpHandler),
		})),
	)

	handle("/stamps/dilute/{batch_id}/{depth}", web.ChainHandlers(
		s.checkChainAvailability,
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

	handle("/redistributionstate", web.ChainHandlers(
		s.checkStorageIncentivesAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.redistributionStatusHandler),
		})),
	)

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
