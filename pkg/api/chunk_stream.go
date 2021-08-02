// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/websocket"
)

var successWsMsg = []byte{}

func (s *server) chunkUploadStreamHandler(w http.ResponseWriter, r *http.Request) {

	ctx, tag, putter, err := s.processUploadRequest(r)
	if err != nil {
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkSize,
		WriteBufferSize: swarm.ChunkSize,
		CheckOrigin:     s.checkOrigin,
	}

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Debugf("chunk stream handler failed upgrading: %v", err)
		s.logger.Error("chunk stream handler: upgrading")
		jsonhttp.BadRequest(w, "not a websocket connection")
		return
	}

	s.wsWg.Add(1)
	go s.handleUploadStream(
		ctx,
		c,
		tag,
		putter,
		requestModePut(r),
		strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true",
	)
}

func (s *server) handleUploadStream(
	ctx context.Context,
	conn *websocket.Conn,
	tag *tags.Tag,
	putter storage.Putter,
	mode storage.ModePut,
	pin bool,
) {
	defer s.wsWg.Done()

	var (
		gone = make(chan struct{})
		err  error
	)
	defer func() {
		_ = conn.Close()
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debugf("chunk stream handler: client gone. code %d message %s", code, text)
		close(gone)
		return nil
	})

	sendMsg := func(msgType int, buf []byte) error {
		err := conn.SetWriteDeadline(time.Now().Add(writeDeadline))
		if err != nil {
			return err
		}
		err = conn.WriteMessage(msgType, buf)
		if err != nil {
			return err
		}
		return nil
	}

	sendErrorClose := func(code int, errmsg string) {
		err := conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, errmsg),
			time.Now().Add(writeDeadline),
		)
		if err != nil {
			s.logger.Errorf("chunk stream handler: failed sending close msg")
		}
	}

	for {
		select {
		case <-s.quit:
			// shutdown
			sendErrorClose(websocket.CloseGoingAway, "node shutting down")
			return
		case <-gone:
			// client gone
			return
		default:
			// if there is no indication to stop, go ahead and read the next message
		}

		err = conn.SetReadDeadline(time.Now().Add(readDeadline))
		if err != nil {
			s.logger.Debugf("chunk stream handler: set read deadline: %v", err)
			s.logger.Error("chunk stream handler: set read deadline")
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			s.logger.Debugf("chunk stream handler: read message error: %v", err)
			s.logger.Error("chunk stream handler: read message error")
			return
		}

		if mt != websocket.BinaryMessage {
			s.logger.Debug("chunk stream handler: unexpected message received from client", mt)
			s.logger.Error("chunk stream handler: unexpected message received from client")
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message")
			return
		}

		if len(msg) < swarm.SpanSize {
			s.logger.Debug("chunk stream handler: not enough data")
			s.logger.Error("chunk stream handler: not enough data")
			return
		}

		chunk, err := cac.NewWithDataSpan(msg)
		if err != nil {
			s.logger.Debugf("chunk stream handler: create chunk error: %v", err)
			s.logger.Error("chunk stream handler: failed creating chunk")
			return
		}

		seen, err := putter.Put(ctx, mode, chunk)
		if err != nil {
			s.logger.Debugf("chunk stream handler: chunk write error: %v, addr %s", err, chunk.Address())
			s.logger.Error("chunk stream handler: chunk write error")
			switch {
			case errors.Is(err, postage.ErrBucketFull):
				sendErrorClose(websocket.CloseInternalServerErr, "batch is overissued")
			default:
				sendErrorClose(websocket.CloseInternalServerErr, "chunk write error")
			}
			return
		} else if len(seen) > 0 && seen[0] && tag != nil {
			err := tag.Inc(tags.StateSeen)
			if err != nil {
				s.logger.Debugf("chunk stream handler: increment tag", err)
				s.logger.Error("chunk stream handler: increment tag")
				sendErrorClose(websocket.CloseInternalServerErr, "failed incrementing tag")
				return
			}
		} else if tag != nil {
			// indicate that the chunk is stored
			err = tag.Inc(tags.StateStored)
			if err != nil {
				s.logger.Debugf("chunk stream handler: increment tag", err)
				s.logger.Error("chunk stream handler: increment tag")
				sendErrorClose(websocket.CloseInternalServerErr, "failed incrementing tag")
				return
			}
		}

		if pin {
			if err := s.pinning.CreatePin(ctx, chunk.Address(), false); err != nil {
				s.logger.Debugf("chunk stream handler: creation of pin for %q failed: %v", chunk.Address(), err)
				s.logger.Error("chunk stream handler: creation of pin failed")
				sendErrorClose(websocket.CloseInternalServerErr, "failed creating pin")
				return
			}
		}

		err = sendMsg(websocket.BinaryMessage, successWsMsg)
		if err != nil {
			s.logger.Debugf("chunk stream handler: failed sending success msg: %v", err)
			s.logger.Error("chunk stream handler: failed sending confirmation")
			return
		}
	}
}
