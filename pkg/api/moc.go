// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/moc"
	"github.com/gorilla/mux"
)

func (s *Service) mocWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("moc_subscribe").Build()

	paths := struct {
		ID []byte `map:"id" validate:"required"`
	}{}

	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	conn, ok := s.wsUpgrade(w, r, logger)
	if !ok {
		return
	}

	s.wsWg.Add(1)
	go s.socSubscribeWs("moc", conn, func(handler func([]byte)) func() {
		return s.moc.Subscribe(paths.ID, moc.Handler(handler))
	})
}
