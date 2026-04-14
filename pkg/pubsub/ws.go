// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/gorilla/websocket"
)

type WsOptions struct {
	PingPeriod time.Duration
	Cancel     context.CancelFunc
}

// ListeningWs bridges a subscriber's p2p stream to a WebSocket connection.
// The Mode on sc.Mode handles all wire-format details: reading broker messages,
// verifying them, and returning the payload to forward to the WebSocket.
// If the subscriber is a Participant, it also reads from the WebSocket
// and writes raw messages to the p2p stream.
func ListeningWs(ctx context.Context, conn *websocket.Conn, options WsOptions, logger log.Logger, sc *SubscriberConn, isParticipant bool) {
	var (
		ticker        = time.NewTicker(options.PingPeriod)
		writeDeadline = options.PingPeriod + time.Second
		readDeadline  = options.PingPeriod + time.Second
	)

	conn.SetCloseHandler(func(code int, text string) error {
		logger.Debug("pubsub ws: client gone", "topic", fmt.Sprintf("%x", sc.TopicAddr), "code", code, "message", text)
		return nil
	})

	// If Participant, read from WebSocket and write to p2p stream (send to Broker).
	if isParticipant {
		go func() {
			for {
				if err := conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
					logger.Debug("pubsub ws: set read deadline failed", "error", err)
					break
				}
				_, p, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
						logger.Debug("pubsub ws: read error", "error", err)
					}
					break
				}

				if err := writeRaw(sc.Stream, p); err != nil {
					logger.Debug("pubsub ws: write to p2p stream failed", "error", err)
					break
				}
			}
			options.Cancel()
		}()
	}

	// Read from p2p stream (Broker messages) and forward to WebSocket.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			wsPayload, err := sc.Mode.ReadBrokerMessage(sc.Stream)
			if err != nil {
				if ctx.Err() == nil {
					logger.Debug("pubsub ws: read broker message failed", "error", err)
				}
				options.Cancel()
				return
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, wsPayload); err != nil {
				logger.Debug("pubsub ws: write to ws failed", "error", err)
				options.Cancel()
				return
			}
		}
	}()

	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	for {
		if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
			logger.Debug("pubsub ws: set write deadline failed", "error", err)
			return
		}
		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
