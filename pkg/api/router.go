// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"resenje.org/web"
)

func (s *server) setupRouting() {
	baseRouter := mux.NewRouter()

	baseRouter.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User-agent: *\nDisallow: /")
	})

	baseRouter.HandleFunc("/pingpong/{peer-id}", s.pingpongHandler)

	s.Handler = web.ChainHandlers(
		handlers.CompressHandler,
		s.pageviewMetricsHandler,
		web.FinalHandler(baseRouter),
	)
}
