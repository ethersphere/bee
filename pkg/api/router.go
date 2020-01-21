package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/handlers"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	baseRouter := http.NewServeMux()

	baseRouter.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	baseRouter.HandleFunc("/pingpong/", s.pingpongHandler)

	s.Handler = web.ChainHandlers(
		handlers.CompressHandler,
		web.FinalHandler(baseRouter),
	)
}
