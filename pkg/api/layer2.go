// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/layer2"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"golang.org/x/net/context"
)

func (s *Service) layer2WsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("layer2").Build()

	paths := struct {
		StreamName string `map:"streamName" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	if s.beeMode == DevMode {
		logger.Warning("layer2 endpoint is disabled in dev mode")
		jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkWithSpanSize * 128,
		WriteBufferSize: swarm.ChunkWithSpanSize * 128,
		CheckOrigin:     s.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("upgrade failed", "error", err)
		logger.Error(nil, "upgrade failed")
		jsonhttp.InternalServerError(w, "upgrade failed")
		return
	}

	pingPeriod := 100 * time.Second //TODO pass it in header
	ctx, cancel := context.WithCancel(context.Background())
	protocolService := s.l2p2p.GetProtocol(ctx, paths.StreamName)
	err = s.p2p.AddProtocol(protocolService.Protocol())
	if err != nil {
		logger.Error(err, "upgrade failed")
		jsonhttp.InternalServerError(w, "upgrade failed")
		return
	}

	s.wsWg.Add(1)
	go func() {
		layer2.ListeningWs(ctx, conn, layer2.WsOptions{PingPeriod: pingPeriod}, logger, protocolService)
		s.wsWg.Done()
		cancel()
	}()
}
