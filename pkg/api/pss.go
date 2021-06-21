// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"io/ioutil"
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

var (
	writeDeadline   = 4 * time.Second // write deadline. should be smaller than the shutdown timeout on api close
	targetMaxLength = 2               // max target length in bytes, in order to prevent grieving by excess computation
)

func (s *server) pssPostHandler(w http.ResponseWriter, r *http.Request) {
	topicVar := mux.Vars(r)["topic"]
	topic := pss.NewTopic(topicVar)

	targetsVar := mux.Vars(r)["targets"]
	var targets pss.Targets
	tgts := strings.Split(targetsVar, ",")

	for _, v := range tgts {
		target, err := hex.DecodeString(v)
		if err != nil {
			s.logger.Debugf("pss send: bad target (%s): %v", target, err)
			s.logger.Errorf("pss send: bad target (%s): %v", target, err)
			jsonhttp.BadRequest(w, nil)
			return
		}
		if len(target) > targetMaxLength {
			s.logger.Debugf("pss send: bad target length: %d", len(target))
			s.logger.Errorf("pss send: bad target length: %d", len(target))
			jsonhttp.BadRequest(w, nil)
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
			s.logger.Debugf("pss recipient: %v", err)
			s.logger.Error("pss recipient")
			jsonhttp.BadRequest(w, nil)
			return
		}
	}

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.logger.Debugf("pss read payload: %v", err)
		s.logger.Error("pss read payload")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	batch, err := requestPostageBatchId(r)
	switch {
	case errors.Is(err, errSwarmPostageBatchIDHeaderNotFound) && s.post.DefaultIssuer() != nil:
		batch = s.post.DefaultIssuer().ID()
	case err != nil:
		s.logger.Debugf("pss: postage batch id: %v", err)
		s.logger.Error("pss: postage batch id")
		jsonhttp.BadRequest(w, "invalid postage batch id")
		return
	}
	i, err := s.post.GetStampIssuer(batch)
	if err != nil {
		s.logger.Debugf("pss: postage batch issuer: %v", err)
		s.logger.Error("pss: postage batch issue")
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
		s.logger.Debugf("pss send payload: %v. topic: %s", err, topicVar)
		s.logger.Error("pss send payload")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, nil)
		}
		return
	}

	jsonhttp.Created(w, nil)
}

func (s *server) pssWsHandler(w http.ResponseWriter, r *http.Request) {

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkSize,
		WriteBufferSize: swarm.ChunkSize,
		CheckOrigin:     s.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Debugf("pss ws: upgrade: %v", err)
		s.logger.Error("pss ws: cannot upgrade")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	t := mux.Vars(r)["topic"]
	s.wsWg.Add(1)
	go s.pumpWs(conn, t)
}

func (s *server) pumpWs(conn *websocket.Conn, t string) {
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
		s.logger.Debugf("pss handler: client gone. code %d message %s", code, text)
		close(gone)
		return nil
	})

	for {
		select {
		case b := <-dataC:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debugf("pss set write deadline: %v", err)
				return
			}

			err = conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				s.logger.Debugf("pss write to websocket: %v", err)
				return
			}

		case <-s.quit:
			// shutdown
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debugf("pss set write deadline: %v", err)
				return
			}
			err = conn.WriteMessage(websocket.CloseMessage, []byte{})
			if err != nil {
				s.logger.Debugf("pss write close message: %v", err)
			}
			return
		case <-gone:
			// client gone
			return
		case <-ticker.C:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debugf("pss set write deadline: %v", err)
				return
			}
			if err = conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// error encountered while pinging client. client probably gone
				return
			}
		}
	}
}
