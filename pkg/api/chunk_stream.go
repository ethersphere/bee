// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/websocket"
)

const streamReadTimeout = 15 * time.Minute

var successWsMsg = []byte{}

func (s *Service) chunkUploadStreamHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("chunks_stream").Build()

	headers := struct {
		BatchID  []byte `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag uint64 `map:"Swarm-Tag"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	var (
		tag uint64
		err error
	)
	if headers.SwarmTag > 0 {
		tag, err = s.getOrCreateSessionID(headers.SwarmTag)
		if err != nil {
			logger.Debug("unable to get/create session", "tag", headers.SwarmTag, "error", err)
			logger.Warning("unable to get/create session")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "cannot get or create tag")
			}
			return
		}
	}

	// if tag not specified use direct upload
	deferred := tag != 0
	putter, err := s.newStamperPutter(r.Context(), putterOptions{
		BatchID:  headers.BatchID,
		TagID:    tag,
		Deferred: deferred,
	})
	if err != nil {
		logger.Debug("unable to get putter", "batch_id", headers.BatchID, "tag_id", tag, "deferred", deferred, "error", err)
		logger.Warning("unable to get putter")
		switch {
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		case errors.Is(err, errUnsupportedDevNodeOperation):
			jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		default:
			jsonhttp.BadRequest(w, nil)
		}
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.SocMaxChunkSize,
		WriteBufferSize: swarm.SocMaxChunkSize,
		CheckOrigin:     s.checkOrigin,
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("unable to upgrade chunk", "error", err)
		logger.Warning("unable to upgrade chunk")
		jsonhttp.BadRequest(w, "upgrade chunk")
		return
	}

	s.wsWg.Add(1)
	go s.handleUploadStream(logger, wsConn, putter)
}

func (s *Service) handleUploadStream(
	logger log.Logger,
	conn *websocket.Conn,
	putter storer.PutterSession,
) {
	defer s.wsWg.Done()

	ctx, cancel := context.WithCancel(context.Background())

	var (
		gone = make(chan struct{})
		err  error
	)
	defer func() {
		cancel()
		_ = conn.Close()
		if err = putter.Done(swarm.ZeroAddress); err != nil {
			logger.Debug("unable to syncing chunks", "error", err)
		}
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		logger.Debug("client gone", "code", code, "message", text)
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
			logger.Debug("unable to send close message", "error", err)
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

		err = conn.SetReadDeadline(time.Now().Add(streamReadTimeout))
		if err != nil {
			logger.Debug("unable to set read deadline", "error", err)
			logger.Warning("unable to set read deadline")
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Debug("unable to read message", "error", err)
				logger.Warning("unable to read message")
			}
			return
		}

		if mt != websocket.BinaryMessage {
			logger.Debug("unexpected message received from client", "message_type", mt)
			logger.Warning("unexpected message received from client")
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message")
			return
		}

		if len(msg) < swarm.SpanSize {
			logger.Debug("insufficient data", "message_length", len(msg))
			logger.Warning("insufficient data")
			return
		}

		chunk, err := cac.NewWithDataSpan(msg)
		if err != nil {
			logger.Debug("unable to create chunk", "error", err)
			logger.Warning("unable to create chunk")
			return
		}

		err = putter.Put(ctx, chunk)
		if err != nil {
			logger.Debug("unable to write chunk", "address", chunk.Address(), "error", err)
			logger.Warning("unable to write chunk")
			switch {
			case errors.Is(err, postage.ErrBucketFull):
				sendErrorClose(websocket.CloseInternalServerErr, "batch is overissued")
			default:
				sendErrorClose(websocket.CloseInternalServerErr, "chunk write error")
			}
			return
		}

		err = sendMsg(websocket.BinaryMessage, successWsMsg)
		if err != nil {
			logger.Debug("unable to send success message", "error", err)
			logger.Warning("unable to send success message")
			return
		}
	}
}
