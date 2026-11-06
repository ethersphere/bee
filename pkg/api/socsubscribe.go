// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/websocket"
)

// wsUpgrade upgrades an HTTP request to a websocket connection using the
// standard chunk-sized buffers and origin check. On failure it writes the
// error response and returns ok=false.
func (s *Service) wsUpgrade(w http.ResponseWriter, r *http.Request, logger log.Logger) (*websocket.Conn, bool) {
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
		return nil, false
	}

	return conn, true
}

// socSubscribeWs pumps payloads from a single-owner-chunk subscription to the
// websocket connection until the client disconnects or the node shuts down.
// subscribe registers the supplied message handler with the relevant listener
// and returns its cleanup func. name is used for logging only.
func (s *Service) socSubscribeWs(name string, conn *websocket.Conn, subscribe func(handler func([]byte)) (cleanup func())) {
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

	cleanup := subscribe(func(m []byte) {
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
		s.logger.Debug("soc subscribe ws: client gone", "name", name, "code", code, "message", text)
		close(gone)
		return nil
	})

	for {
		select {
		case b := <-dataC:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("soc subscribe ws: set write deadline failed", "name", name, "error", err)
				return
			}

			err = conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				s.logger.Debug("soc subscribe ws: write message failed", "name", name, "error", err)
				return
			}

		case <-s.quit:
			// shutdown
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("soc subscribe ws: set write deadline failed", "name", name, "error", err)
				return
			}
			err = conn.WriteMessage(websocket.CloseMessage, []byte{})
			if err != nil {
				s.logger.Debug("soc subscribe ws: write close message failed", "name", name, "error", err)
			}
			return
		case <-gone:
			// client gone
			return
		case <-ticker.C:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("soc subscribe ws: set write deadline failed", "name", name, "error", err)
				return
			}
			if err = conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// error encountered while pinging client. client probably gone
				return
			}
		}
	}
}
