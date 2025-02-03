// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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
			logger.Debug("get or create tag failed", "error", err)
			logger.Error(nil, "get or create tag failed")
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
	// Using context.Background here because the putter's lifetime extends beyond that of the HTTP request.
	putter, err := s.newStamperPutter(context.Background(), putterOptions{
		BatchID:  headers.BatchID,
		TagID:    tag,
		Deferred: tag != 0,
	})
	if err != nil {
		logger.Debug("get putter failed", "error", err)
		logger.Error(nil, "get putter failed")
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
		logger.Debug("chunk upload: upgrade failed", "error", err)
		logger.Error(nil, "chunk upload: upgrade failed")
		jsonhttp.BadRequest(w, "upgrade failed")
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
			logger.Error(err, "chunk upload stream: syncing chunks failed")
		}
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		logger.Debug("chunk upload stream: client gone", "code", code, "message", text)
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
			logger.Error(err, "chunk upload stream: failed sending close message")
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
			logger.Debug("chunk upload stream: set read deadline failed", "error", err)
			logger.Error(nil, "chunk upload stream: set read deadline failed")
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Debug("chunk upload stream: read message failed", "error", err)
				logger.Error(nil, "chunk upload stream: read message failed")
			}
			return
		}

		if mt != websocket.BinaryMessage {
			logger.Debug("chunk upload stream: unexpected message received from client", "message_type", mt)
			logger.Error(nil, "chunk upload stream: unexpected message received from client")
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message")
			return
		}

		if len(msg) < swarm.SpanSize {
			logger.Debug("chunk upload stream: insufficient data")
			logger.Error(nil, "chunk upload stream: insufficient data")
			return
		}

		chunk, err := cac.NewWithDataSpan(msg)
		if err != nil {
			logger.Debug("chunk upload stream: create chunk failed", "error", err)
			logger.Error(nil, "chunk upload stream: create chunk failed")
			return
		}

		err = putter.Put(ctx, chunk)
		if err != nil {
			logger.Debug("chunk upload stream: write chunk failed", "address", chunk.Address(), "error", err)
			logger.Error(nil, "chunk upload stream: write chunk failed")
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
			s.logger.Debug("chunk upload stream: sending success message failed", "error", err)
			s.logger.Error(nil, "chunk upload stream: sending success message failed")
			return
		}
	}
}
