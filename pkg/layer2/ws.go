// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pingpong exposes the simple ping-pong protocol
// which measures round-trip-time with other peers.
package layer2

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/websocket"
)

type WsOptions struct {
	PingPeriod time.Duration
	Cancel     context.CancelFunc
}

type actionType uint8

const (
	atUnicast actionType = iota
	atBroadcast
)

type responseType uint8

const (
	atMsg responseType = iota
)

func ListeningWs(ctx context.Context, conn *websocket.Conn, options WsOptions, logger log.Logger, protocolService *protocolService) {
	var (
		ticker        = time.NewTicker(options.PingPeriod)
		writeDeadline = options.PingPeriod + 100*time.Millisecond // write deadline. should be smaller than the shutdown timeout on api close
		readDeadline  = options.PingPeriod + 100*time.Millisecond // write deadline. should be smaller than the shutdown timeout on api close
		err           error
	)

	respMessageType := atomic.Uint32{} // 1 textbase, 2 bytebase
	respMessageType.Store(1)
	protocolListener := func(a swarm.Address, b []byte) {
		if respMessageType.Load() == 1 {
			space := byte(' ')
			msg := append([]byte{byte(atMsg + '0'), space}, ([]byte(a.String()))...)
			msg = append(msg, append([]byte{space}, b...)...)
			err := conn.WriteMessage(1, msg)
			if err != nil {
				logger.Error(err, "L2 ws write message")
			}
		} else {
			msg := append([]byte{byte(atMsg + '0')}, a.Bytes()...)
			msg = append(msg, b...)
			err := conn.WriteMessage(2, msg)
			if err != nil {
				logger.Error(err, "L2 ws write message")
			}
		}
	}

	protocolService.AddHandler(protocolListener)
	conn.SetCloseHandler(func(code int, text string) error {
		logger.Debug("L2 ws: client gone", "protocol", protocolService.streamName, "code", code, "message", text)
		protocolService.RemoveHandler(protocolListener)
		return nil
	})

	go func() {
		for {
			err = conn.SetReadDeadline(time.Now().Add(readDeadline))
			if err != nil {
				logger.Debug("L2 ws: set write deadline failed", "error", err)
				break
			}
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				logger.Error(err, "L2 ws read message")
				break
			}
			respMessageType.Store(uint32(messageType))
			if messageType == 1 {
				action := actionType(p[0] - '0')
				offset := 2 // + 1 delimeter
				if action == atUnicast {
					overlayBytes := p[offset : swarm.HashSize*2+offset]
					overlay := swarm.NewAddress(common.HexToHash(string(overlayBytes)).Bytes())
					offset += swarm.HashSize*2 + 1 // + 1 delimeter
					msg := p[offset:]
					conn, err := protocolService.GetConnection(ctx, overlay)
					if err != nil {
						logger.Error(err, "L2 get connection")
						break
					}
					err = conn.SendMessage(ctx, msg)
					if err != nil {
						logger.Error(err, "L2 write message")
					}
				} else if action == atBroadcast {
					msg := p[offset:]
					protocolService.Broadcast(ctx, msg)
				}
			} else if messageType == 2 {
				action := actionType(p[0])
				offset := 1
				if action == atUnicast {
					overlayBytes := p[offset : swarm.HashSize+offset]
					overlay := swarm.NewAddress(overlayBytes)
					offset += swarm.HashSize
					msg := p[offset:]
					conn, err := protocolService.GetConnection(ctx, overlay)
					if err != nil {
						logger.Error(err, "L2 get connection")
					}
					err = conn.SendMessage(ctx, msg)
					if err != nil {
						logger.Error(err, "L2 write message")
					}
				} else if action == atBroadcast {
					msg := p[offset:]

					protocolService.Broadcast(ctx, msg)
				}
			}
		}
		options.Cancel()
	}()

	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	for {
		err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
		if err != nil {
			logger.Debug("L2 ws: set write deadline failed", "error", err)
			return
		}
		select {
		case <-ctx.Done():
			err = conn.WriteMessage(websocket.CloseMessage, []byte{})
			if err != nil {
				logger.Debug("L2 ws: write close message failed", "error", err)
			}
			return
		case <-ticker.C:
			if err != nil {
				logger.Debug("L2 ws: set write deadline failed", "error", err)
				return
			}
			if err = conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// error encountered while pinging client. client probably gone
				return
			}
		}
	}
}
