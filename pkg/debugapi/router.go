package debugapi

import (
	"expvar"
	"net/http"
	"net/http/pprof"

	"github.com/gorilla/handlers"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	internalBaseRouter := http.NewServeMux()

	internalRouter := http.NewServeMux()
	internalBaseRouter.Handle("/", web.ChainHandlers(
		handlers.CompressHandler,
		web.NoCacheHeadersHandler,
		web.FinalHandler(internalRouter),
	))
	internalRouter.Handle("/", http.NotFoundHandler())

	internalRouter.HandleFunc("/health", s.statusHandler)
	internalRouter.HandleFunc("/readiness", s.statusHandler)

	internalRouter.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	internalRouter.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	internalRouter.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	internalRouter.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	internalRouter.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	internalRouter.Handle("/debug/vars", expvar.Handler())

	s.Handler = internalBaseRouter
}
