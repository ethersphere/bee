//go:build js
// +build js

package api

import (
	"expvar"
	"fmt"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log/httpaccess"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"resenje.org/web"
)

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
			s.downloadSpeedMetricMiddleware("bzz"),
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
