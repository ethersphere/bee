//go:build js

package api

import (
	"fmt"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"resenje.org/web"
)

func (s *Service) mountTechnicalDebug() {

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

	handle("/chequebook/balance", web.ChainHandlers(
		s.checkChequebookAvailability,
		web.FinalHandler(jsonhttp.MethodHandler{
			"GET": http.HandlerFunc(s.chequebookBalanceHandler),
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

	handle("/batches", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.postageGetAllBatchesHandler),
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
			s.downloadSpeedMetricMiddleware("bytes"),
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
			s.downloadSpeedMetricMiddleware("bzz"),
			web.FinalHandlerFunc(s.bzzDownloadHandler),
		),
		"HEAD": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bzzHeadHandler),
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
