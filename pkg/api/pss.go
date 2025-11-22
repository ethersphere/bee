// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

const (
	writeDeadline   = 4 * time.Second // write deadline. should be smaller than the shutdown timeout on api close
	targetMaxLength = 3               // max target length in bytes, in order to prevent grieving by excess computation
)

func (s *Service) pssPostHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_pss_send").Build()

	paths := struct {
		Topic   string `map:"topic" validate:"required"`
		Targets string `map:"targets" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}
	topic := pss.NewTopic(paths.Topic)

	var targets pss.Targets
	for _, v := range strings.Split(paths.Targets, ",") {
		target := struct {
			Val []byte `map:"target" validate:"required,max=3"`
		}{}
		if response := s.mapStructure(map[string]string{"target": v}, &target); response != nil {
			response("invalid path params", logger, w)
			return
		}
		targets = append(targets, target.Val)
	}

	queries := struct {
		Recipient *ecdsa.PublicKey `map:"recipient,omitempty"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}
	if queries.Recipient == nil {
		queries.Recipient = &(crypto.Secp256k1PrivateKeyFromBytes(topic[:])).PublicKey
	}

	headers := struct {
		BatchID []byte `map:"Swarm-Postage-Batch-Id" validate:"required"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Debug("read body failed", "error", err)
		logger.Error(nil, "read body failed")
		jsonhttp.InternalServerError(w, "pss send failed")
		return
	}
	i, save, err := s.post.GetStampIssuer(headers.BatchID)
	if err != nil {
		logger.Debug("get postage batch issuer failed", "batch_id", hex.EncodeToString(headers.BatchID), "error", err)
		logger.Error(nil, "get postage batch issuer failed")
		switch {
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.BadRequest(w, "batch not found")
		case errors.Is(err, postage.ErrNotUsable):
			jsonhttp.BadRequest(w, "batch not usable yet")
		default:
			jsonhttp.BadRequest(w, "postage stamp issuer")
		}
		return
	}

	stamper := postage.NewStamper(s.stamperStore, i, s.signer)

	err = s.pss.Send(r.Context(), topic, payload, stamper, queries.Recipient, targets)
	if err != nil {
		logger.Debug("send payload failed", "topic", paths.Topic, "error", err)
		logger.Error(nil, "send payload failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "pss send failed")
		}
		return
	}

	if err = save(); err != nil {
		logger.Debug("save stamp failed", "error", err)
		logger.Error(nil, "save stamp failed")
		jsonhttp.InternalServerError(w, "pss send failed")
		return
	}

	jsonhttp.Created(w, nil)
}

func (s *Service) pssWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("pss_subscribe").Build()

	paths := struct {
		Topic string `map:"topic" validate:"required"`
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
	go s.pumpWs(conn, paths.Topic)
}

func (s *Service) pumpWs(conn *websocket.Conn, t string) {
	defer s.wsWg.Done()

	var (
		dataC  = make(chan []byte)
		gone   = make(chan struct{})
		topic  = pss.NewTopic(t)
		ticker = time.NewTicker(s.WsPingPeriod)
		err    error
	)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	cleanup := s.pss.Register(topic, func(ctx context.Context, m []byte) {
		select {
		case dataC <- m:
		case <-ctx.Done():
			return
		case <-gone:
			return
		case <-s.quit:
			return
		}
	})

	defer cleanup()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debug("pss ws: client gone", "code", code, "message", text)
		close(gone)
		return nil
	})

	for {
		select {
		case b := <-dataC:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("pss ws: set write deadline failed", "error", err)
				return
			}

			err = conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				s.logger.Debug("pss ws: write message failed", "error", err)
				return
			}

		case <-s.quit:
			// shutdown
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("pss ws: set write deadline failed", "error", err)
				return
			}
			err = conn.WriteMessage(websocket.CloseMessage, []byte{})
			if err != nil {
				s.logger.Debug("pss ws: write close message failed", "error", err)
			}
			return
		case <-gone:
			// client gone
			return
		case <-ticker.C:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("pss ws: set write deadline failed", "error", err)
				return
			}
			if err = conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// error encountered while pinging client. client probably gone
				return
			}
		}
	}
}
