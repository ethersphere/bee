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
// If the subscriber is a Publisher, it also reads from the WebSocket
// and writes raw messages to the p2p stream.
func ListeningWs(ctx context.Context, conn *websocket.Conn, options WsOptions, logger log.Logger, sc *SubscriberConn, isPublisher bool) {
	var (
		ticker        = time.NewTicker(options.PingPeriod)
		writeDeadline = options.PingPeriod + time.Second
		readDeadline  = options.PingPeriod + time.Second
	)

	logger.Info("pubsub ws: starting", "topic", fmt.Sprintf("%x", sc.TopicAddr), "isPublisher", isPublisher, "pingPeriod", options.PingPeriod)

	conn.SetCloseHandler(func(code int, text string) error {
		logger.Info("pubsub ws: client gone", "topic", fmt.Sprintf("%x", sc.TopicAddr), "code", code, "message", text)
		return nil
	})

	// A read loop is always required so gorilla can process pong responses
	// and close frames from the client.
	go func() {
		for {
			if err := conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
				logger.Info("pubsub ws: set read deadline failed", "error", err)
				break
			}
			msgType, p, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					logger.Info("pubsub ws: read error", "error", err)
				} else {
					logger.Info("pubsub ws: read loop ended", "error", err)
				}
				break
			}

			if isPublisher {
				logger.Info("pubsub ws: publisher message from ws", "type", msgType, "size", len(p))
				if err := writeRaw(sc.Stream, p); err != nil {
					logger.Info("pubsub ws: write to p2p stream failed", "error", err)
					break
				}
			}
		}
		options.Cancel()
	}()

	// Read from p2p stream (Broker messages) and forward to WebSocket.
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Info("pubsub ws: p2p reader context done")
				return
			default:
			}

			wsPayload, err := sc.Mode.ReadBrokerMessage(sc.Stream)
			if err != nil {
				if ctx.Err() == nil {
					logger.Info("pubsub ws: read broker message failed", "error", err)
				}
				options.Cancel()
				return
			}

			logger.Info("pubsub ws: forwarding broker message to ws", "size", len(wsPayload))
			if err := conn.WriteMessage(websocket.BinaryMessage, wsPayload); err != nil {
				logger.Info("pubsub ws: write to ws failed", "error", err)
				options.Cancel()
				return
			}
		}
	}()

	defer func() {
		ticker.Stop()
		_ = conn.Close()
		logger.Info("pubsub ws: closed", "topic", fmt.Sprintf("%x", sc.TopicAddr))
	}()

	for {
		if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
			logger.Info("pubsub ws: set write deadline failed", "error", err)
			return
		}
		select {
		case <-ctx.Done():
			logger.Info("pubsub ws: context cancelled, closing")
			_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.Info("pubsub ws: ping failed, closing", "error", err)
				return
			}
		}
	}
}
