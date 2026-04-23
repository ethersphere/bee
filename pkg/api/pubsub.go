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

	// Required: underlay multiaddr — accept from header or query param (browsers cannot set WS headers)
	peerHeader := r.Header.Get(SwarmPubsubPeerHeader)
	if peerHeader == "" {
		peerHeader = r.URL.Query().Get("swarm-pubsub-peer")
	}
	if peerHeader == "" {
		jsonhttp.BadRequest(w, "missing Swarm-Pubsub-Peer header")
		return
	}
	underlay, err := ma.NewMultiaddr(peerHeader)
	if err != nil {
		logger.Info("invalid peer multiaddr", "value", peerHeader, "error", err)
		jsonhttp.BadRequest(w, "invalid Swarm-Pubsub-Peer header")
		return
	}

	// Optional: GSOC fields for Publisher upgrade — accept from header or query param
	var connectOpts pubsub.ConnectOptions

	gsocEthAddrHex := r.Header.Get(SwarmPubsubGsocEthAddressHeader)
	if gsocEthAddrHex == "" {
		gsocEthAddrHex = r.URL.Query().Get("swarm-pubsub-gsoc-eth-address")
	}
	gsocTopicHex := r.Header.Get(SwarmPubsubGsocTopicHeader)
	if gsocTopicHex == "" {
		gsocTopicHex = r.URL.Query().Get("swarm-pubsub-gsoc-topic")
	}
	if gsocEthAddrHex != "" && gsocTopicHex != "" {
		gsocOwner, err := hex.DecodeString(gsocEthAddrHex)
		if err != nil {
			jsonhttp.BadRequest(w, "invalid Swarm-Pubsub-Gsoc-Eth-Address header")
			return
		}
		gsocID, err := hex.DecodeString(gsocTopicHex)
		if err != nil {
			jsonhttp.BadRequest(w, "invalid Swarm-Pubsub-Gsoc-Topic header")
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

	if s.beeMode == DevMode {
		logger.Warning("pubsub endpoint is disabled in dev mode")
		jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
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
		if sc := mode.GetSubscriberConn(); sc != nil {
			_ = sc.Stream.Close()
		}
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
