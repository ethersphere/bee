// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

const (
	writeDeadline   = 4 * time.Second // write deadline. should be smaller than the shutdown timeout on api close
	targetMaxLength = 3               // max target length in bytes, in order to prevent grieving by excess computation
)

func (s *Service) pssPostHandler(w http.ResponseWriter, r *http.Request) {
	topicVar := mux.Vars(r)["topic"]
	topic := pss.NewTopic(topicVar)

	targetsVar := mux.Vars(r)["targets"]
	var targets pss.Targets
	tgts := strings.Split(targetsVar, ",")

	for _, v := range tgts {
		target, err := hex.DecodeString(v)
		if err != nil {
			s.logger.Debug("pss post: decode target string failed", "string", target, "error", err)
			s.logger.Error(nil, "pss post: decode target string failed", "string", target)
			jsonhttp.BadRequest(w, "target is not valid hex string")
			return
		}
		if len(target) > targetMaxLength {
			s.logger.Debug("pss post: invalid target string length", "string", target, "length", len(target))
			s.logger.Error(nil, "pss post: invalid target string length", "string", target, "length", len(target))
			jsonhttp.BadRequest(w, fmt.Sprintf("hex string target exceeds max length of %d", targetMaxLength*2))
			return
		}
		targets = append(targets, target)
	}

	recipientQueryString := r.URL.Query().Get("recipient")
	var recipient *ecdsa.PublicKey
	if recipientQueryString == "" {
		// use topic-based encryption
		privkey := crypto.Secp256k1PrivateKeyFromBytes(topic[:])
		recipient = &privkey.PublicKey
	} else {
		var err error
		recipient, err = pss.ParseRecipient(recipientQueryString)
		if err != nil {
			s.logger.Debug("pss post: parse recipient string failed", "string", recipientQueryString, "error", err)
			s.logger.Error(nil, "pss post: parse recipient string failed")
			jsonhttp.BadRequest(w, "pss recipient: invalid format")
			return
		}
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Debug("pss post: read body failed", "error", err)
		s.logger.Error(nil, "pss post: read body failed")
		jsonhttp.InternalServerError(w, "pss send failed")
		return
	}
	batch, err := requestPostageBatchId(r)
	if err != nil {
		s.logger.Debug("pss post: decode postage batch id failed", "error", err)
		s.logger.Error(nil, "pss post: decode postage batch id failed")
		jsonhttp.BadRequest(w, "invalid postage batch id")
		return
	}
	i, err := s.post.GetStampIssuer(batch)
	if err != nil {
		s.logger.Debug("pss post: get postage batch issuer failed", "batch_id", batch, "error", err)
		s.logger.Error(nil, "pss post: get postage batch issuer failed")
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
	stamper := postage.NewStamper(i, s.signer)

	err = s.pss.Send(r.Context(), topic, payload, stamper, recipient, targets)
	if err != nil {
		s.logger.Debug("pss post: send payload failed", "topic", fmt.Sprintf("%x", topic), "error", err)
		s.logger.Error(nil, "pss post: send payload failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "pss send failed")
		}
		return
	}

	jsonhttp.Created(w, nil)
}

func (s *Service) pssWsHandler(w http.ResponseWriter, r *http.Request) {

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkSize,
		WriteBufferSize: swarm.ChunkSize,
		CheckOrigin:     s.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Debug("pss ws: upgrade failed", "error", err)
		s.logger.Error(nil, "pss ws: upgrade failed")
		jsonhttp.InternalServerError(w, "pss ws: upgrade failed")
		return
	}

	t := mux.Vars(r)["topic"]
	s.wsWg.Add(1)
	go s.pumpWs(conn, t)
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
	cleanup := s.pss.Register(topic, func(_ context.Context, m []byte) {
		dataC <- m
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
