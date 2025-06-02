//go:build js
// +build js

package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/handlers"
	"resenje.org/web"
)

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
		s.corsHandler,
		web.FinalHandler(s.router),
	)
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
			web.FinalHandlerFunc(s.bytesUploadHandler),
		),
	})

	handle("/bytes/{address}", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bytesGetHandler),
		),
		"HEAD": web.ChainHandlers(
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
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bzzDownloadHandler),
		),
		"HEAD": web.ChainHandlers(
			s.actDecryptionHandler(),
			web.FinalHandlerFunc(s.bzzHeadHandler),
		),
	})

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
func (s *Service) mountTechnicalDebug() {
	s.router.Handle("/node", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.nodeGetHandler),
	})

	s.router.Handle("/addresses", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.addressesHandler),
	})

	s.router.Handle("/debugstore", jsonhttp.MethodHandler{
		"GET": web.ChainHandlers(
			httpaccess.NewHTTPAccessSuppressLogHandler(),
			web.FinalHandlerFunc(s.debugStorage),
		),
	})

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

	handle("/peers", jsonhttp.MethodHandler{
		"GET": http.HandlerFunc(s.peersHandler),
	})

	handle("/pingpong/{address}", jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(s.pingpongHandler),
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
