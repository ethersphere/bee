// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

func (s *Service) gsocWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("gsoc_subscribe").Build()

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}

	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkSize,
		WriteBufferSize: swarm.ChunkSize,
		CheckOrigin:     s.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("upgrade failed", "error", err)
		logger.Error(nil, "upgrade failed")
		jsonhttp.InternalServerError(w, "upgrade failed")
		return
	}

	s.wsWg.Add(1)
	go s.gsocListeningWs(conn, paths.Address)
}

func (s *Service) gsocListeningWs(conn *websocket.Conn, socAddress swarm.Address) {
	defer s.wsWg.Done()

	var (
		dataC  = make(chan []byte)
		gone   = make(chan struct{})
		ticker = time.NewTicker(s.WsPingPeriod)
		err    error
	)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	cleanup := s.gsoc.Subscribe(socAddress, func(m []byte) {
		select {
		case dataC <- m:
		case <-gone:
			return
		case <-s.quit:
			return
		}
	})

	defer cleanup()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debug("gsoc ws: client gone", "code", code, "message", text)
		close(gone)
		return nil
	})

	for {
		select {
		case b := <-dataC:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("gsoc ws: set write deadline failed", "error", err)
				return
			}

			err = conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				s.logger.Debug("gsoc ws: write message failed", "error", err)
				return
			}

		case <-s.quit:
			// shutdown
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("gsoc ws: set write deadline failed", "error", err)
				return
			}
			err = conn.WriteMessage(websocket.CloseMessage, []byte{})
			if err != nil {
				s.logger.Debug("gsoc ws: write close message failed", "error", err)
			}
			return
		case <-gone:
			// client gone
			return
		case <-ticker.C:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("gsoc ws: set write deadline failed", "error", err)
				return
			}
			if err = conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// error encountered while pinging client. client probably gone
				return
			}
		}
	}
}
