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
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/websocket"
)

const streamReadTimeout = 15 * time.Minute

var successWsMsg = []byte{}

func (s *Service) chunkUploadStreamHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("chunks_stream").Build()

	_, tag, putter, wait, err := s.processUploadRequest(logger, r)
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
		logger.Debug("chunk upload: upgrade failed", "error", err)
		logger.Error(nil, "chunk upload: upgrade failed")
		jsonhttp.BadRequest(w, "upgrade failed")
		return
	}

	cctx := context.Background()
	if tag != nil {
		cctx = sctx.SetTag(cctx, tag)
	}

	s.wsWg.Add(1)
	go s.handleUploadStream(
		cctx,
		c,
		tag,
		putter,
		requestModePut(r),
		requestPin(r),
		wait,
	)
}

func (s *Service) handleUploadStream(
	ctx context.Context,
	conn *websocket.Conn,
	tag *tags.Tag,
	putter storage.Putter,
	mode storage.ModePut,
	pin bool,
	wait func() error,
) {
	defer s.wsWg.Done()

	var (
		gone = make(chan struct{})
		err  error
	)
	defer func() {
		_ = conn.Close()
		if err = wait(); err != nil {
			s.logger.Error(err, "chunk upload stream: syncing chunks failed")
		}
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debug("chunk upload stream: client gone", "code", code, "message", text)
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
			s.logger.Error(err, "chunk upload stream: failed sending close message")
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
			s.logger.Debug("chunk upload stream: set read deadline failed", "error", err)
			s.logger.Error(nil, "chunk upload stream: set read deadline failed")
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Debug("chunk upload stream: read message failed", "error", err)
				s.logger.Error(nil, "chunk upload stream: read message failed")
			}
			return
		}

		if mt != websocket.BinaryMessage {
			s.logger.Debug("chunk upload stream: unexpected message received from client", "message_type", mt)
			s.logger.Error(nil, "chunk upload stream: unexpected message received from client")
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message")
			return
		}

		if tag != nil {
			err = tag.Inc(tags.StateSplit)
			if err != nil {
				s.logger.Debug("chunk upload stream: incrementing tag failed", "error", err)
				s.logger.Error(nil, "chunk upload stream: incrementing tag failed")
				sendErrorClose(websocket.CloseInternalServerErr, "incrementing tag failed")
				return
			}
		}

		if len(msg) < swarm.SpanSize {
			s.logger.Debug("chunk upload stream: insufficient data")
			s.logger.Error(nil, "chunk upload stream: insufficient data")
			return
		}

		chunk, err := cac.NewWithDataSpan(msg)
		if err != nil {
			s.logger.Debug("chunk upload stream: create chunk failed", "error", err)
			s.logger.Error(nil, "chunk upload stream: create chunk failed")
			return
		}

		seen, err := putter.Put(ctx, mode, chunk)
		if err != nil {
			s.logger.Debug("chunk upload stream: write chunk failed", "address", chunk.Address(), "error", err)
			s.logger.Error(nil, "chunk upload stream: write chunk failed")
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
				s.logger.Debug("chunk upload stream: increment tag failed", "error", err)
				s.logger.Error(nil, "chunk upload stream: increment tag")
				sendErrorClose(websocket.CloseInternalServerErr, "incrementing tag failed")
				return
			}
		}

		if tag != nil {
			// indicate that the chunk is stored
			err = tag.Inc(tags.StateStored)
			if err != nil {
				s.logger.Debug("chunk upload stream: increment tag failed", "error", err)
				s.logger.Error(nil, "chunk upload stream: increment tag failed")
				sendErrorClose(websocket.CloseInternalServerErr, "incrementing tag failed")
				return
			}
		}

		if pin {
			if err := s.pinning.CreatePin(ctx, chunk.Address(), false); err != nil {
				s.logger.Debug("chunk upload stream: pin creation failed", "chunk_address", chunk.Address(), "error", err)
				s.logger.Error(nil, "chunk upload stream: pin creation failed")
				// since we already increment the pin counter because of the ModePut, we need
				// to delete the pin here to prevent the pin counter from never going to 0
				err = s.storer.Set(ctx, storage.ModeSetUnpin, chunk.Address())
				if err != nil {
					s.logger.Debug("chunk upload stream: pin deletion failed", "chunk_address", chunk.Address(), "error", err)
					s.logger.Error(nil, "chunk upload stream: pin deletion failed")
				}
				sendErrorClose(websocket.CloseInternalServerErr, "failed creating pin")
				return
			}
		}

		err = sendMsg(websocket.BinaryMessage, successWsMsg)
		if err != nil {
			s.logger.Debug("chunk upload stream: sending success message failed", "error", err)
			s.logger.Error(nil, "chunk upload stream: sending success message failed")
			return
		}
	}
}
