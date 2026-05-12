// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/pubsub"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	ma "github.com/multiformats/go-multiaddr"
)

func (s *Service) pubsubWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("pubsub").Build()

	paths := struct {
		Topic string `map:"topic" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	var topicAddr [32]byte
	if decoded, err := hex.DecodeString(paths.Topic); err == nil && len(decoded) == swarm.HashSize {
		copy(topicAddr[:], decoded)
	} else {
		h := swarm.NewHasher()
		_, _ = h.Write([]byte(paths.Topic))
		copy(topicAddr[:], h.Sum(nil))
	}

	peerMultiaddr := r.URL.Query().Get("peer")
	if peerMultiaddr == "" {
		jsonhttp.BadRequest(w, "missing peer query param")
		return
	}
	underlay, err := ma.NewMultiaddr(peerMultiaddr)
	if err != nil {
		logger.Info("invalid peer multiaddr", "value", peerMultiaddr, "error", err)
		jsonhttp.BadRequest(w, "invalid peer query param")
		return
	}

	var connectOpts pubsub.ConnectOptions

	gsocEthAddrHex := r.URL.Query().Get("gsoc-eth-address")
	gsocTopicHex := r.URL.Query().Get("gsoc-topic")
	if gsocEthAddrHex != "" && gsocTopicHex != "" {
		gsocOwner, err := hex.DecodeString(gsocEthAddrHex)
		if err != nil {
			jsonhttp.BadRequest(w, "invalid gsoc-eth-address query param")
			return
		}
		gsocID, err := hex.DecodeString(gsocTopicHex)
		if err != nil {
			jsonhttp.BadRequest(w, "invalid gsoc-topic query param")
			return
		}
		connectOpts.GsocOwner = gsocOwner
		connectOpts.GsocID = gsocID
		connectOpts.ReadWrite = true
	}

	headers := struct {
		KeepAlive time.Duration `map:"Swarm-Keep-Alive"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	// Connect to broker peer
	ctx, cancel := context.WithCancel(context.Background())
	mode, err := s.pubsubSvc.Connect(ctx, underlay, topicAddr, pubsub.ModeGSOCEphemeral, connectOpts)
	if err != nil {
		cancel()
		logger.Info("pubsub connect failed", "error", err)
		jsonhttp.InternalServerError(w, "pubsub connect failed")
		return
	}
	// Upgrade to WebSocket
	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkWithSpanSize,
		WriteBufferSize: swarm.ChunkWithSpanSize,
		CheckOrigin:     s.checkOrigin,
	}

	logger.Info("upgrading to websocket")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade() hijacks the connection before returning an error,
		// so do NOT write an HTTP response here.
		cancel()
		logger.Info("websocket upgrade failed", "error", err)
		logger.Error(nil, "websocket upgrade failed")
		return
	}
	logger.Info("websocket upgrade successful")

	pingPeriod := headers.KeepAlive * time.Second
	if pingPeriod == 0 {
		pingPeriod = time.Minute
	}

	isPublisher := connectOpts.ReadWrite

	s.wsWg.Add(1)
	go func() {
		pubsub.ListeningWs(ctx, conn, pubsub.WsOptions{PingPeriod: pingPeriod, Cancel: cancel}, logger, mode, isPublisher)
		cancel()
		_ = conn.Close()
		s.wsWg.Done()
	}()
}

func (s *Service) pubsubListHandler(w http.ResponseWriter, r *http.Request) {
	if s.pubsubSvc == nil {
		jsonhttp.NotFound(w, "pubsub service not available")
		return
	}

	topics := s.pubsubSvc.Topics()
	jsonhttp.OK(w, struct {
		Topics []pubsub.TopicInfo `json:"topics"`
	}{
		Topics: topics,
	})
}
