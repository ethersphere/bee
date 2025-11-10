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
		BatchID  []byte `map:"Swarm-Postage-Batch-Id"` // Optional: can be omitted for per-chunk stamping
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

	// Create connection-level putter only if BatchID is provided
	// If BatchID is not provided, per-chunk stamps must be used
	var putter storer.PutterSession
	if len(headers.BatchID) > 0 {
		// if tag not specified use direct upload
		// Using context.Background here because the putter's lifetime extends beyond that of the HTTP request.
		putter, err = s.newStamperPutter(context.Background(), putterOptions{
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
	go s.handleUploadStream(logger, wsConn, putter, tag)
}

func (s *Service) handleUploadStream(
	logger log.Logger,
	conn *websocket.Conn,
	putter storer.PutterSession,
	tag uint64,
) {
	defer s.wsWg.Done()

	ctx, cancel := context.WithCancel(context.Background())

	var (
		gone = make(chan struct{})
		err  error
	)

	// Cache for storage sessions to avoid creating new sessions for every chunk
	// Key: batch ID hex string, Value: storage session (not wrapped with stamper)
	// Note: We cache the underlying session, not the stamped putter, because
	// each chunk has a unique stamp signature and needs its own stamper wrapper
	sessionCache := make(map[string]storer.PutterSession)

	defer func() {
		cancel()
		_ = conn.Close()

		// Clean up all cached sessions
		for batchID, cachedSession := range sessionCache {
			if err := cachedSession.Done(swarm.ZeroAddress); err != nil {
				logger.Error(err, "chunk upload stream: failed to finalize cached session", "batch_id", batchID)
			}
		}

		// Only call Done on connection-level putter if it exists
		if putter != nil {
			if err = putter.Done(swarm.ZeroAddress); err != nil {
				logger.Error(err, "chunk upload stream: syncing chunks failed")
			}
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

		logger.Debug("chunk upload stream",
			"message_size", len(msg),
			"stamp_size", postage.StampSize,
			"first_8_bytes", msg[:8])

		// Check if this message contains a per-chunk stamp prepended to the chunk data
		// Format: stamp[113 bytes] + chunk[data]
		var (
			chunk       swarm.Chunk
			chunkPutter = putter // default to connection-level putter
			chunkData   = msg
		)

		// If message is large enough to contain a stamp + chunk data, try to extract the stamp
		if len(msg) >= postage.StampSize+swarm.SpanSize {
			potentialStamp := msg[:postage.StampSize]
			potentialChunkData := msg[postage.StampSize:]

			logger.Debug("chunk upload stream: attempting to extract stamp",
				"message_size", len(msg),
				"stamp_size", postage.StampSize,
				"potential_stamp_len", len(potentialStamp),
				"potential_chunk_len", len(potentialChunkData),
				"first_8_bytes", msg[:8])

			// Try to unmarshal as a stamp
			stamp := postage.Stamp{}
			if err := stamp.UnmarshalBinary(potentialStamp); err == nil {
				// Valid stamp found - get or create cached session, then wrap with this chunk's stamp
				batchID := stamp.BatchID()
				batchIDHex := string(batchID) // Use batch ID bytes as map key
				logger.Debug("chunk upload stream: per-chunk stamp detected", "batch_id", batchID, "chunk_size", len(potentialChunkData))

				// Check if we already have a cached session for this batch
				cachedSession, exists := sessionCache[batchIDHex]
				if !exists {
					// Validate batch exists
					_, err := s.batchStore.Get(stamp.BatchID())
					if err != nil {
						logger.Debug("chunk upload stream: batch validation failed", "error", err)
						logger.Error(nil, "chunk upload stream: batch validation failed")
						if errors.Is(err, storage.ErrNotFound) {
							sendErrorClose(websocket.CloseInternalServerErr, "batch not found")
						} else {
							sendErrorClose(websocket.CloseInternalServerErr, "batch validation failed")
						}
						return
					}

					// Create a new storage session and cache it
					// For pre-stamped chunks, we use the session directly without a stamper wrapper
					if tag != 0 {
						cachedSession, err = s.storer.Upload(ctx, false, tag)
					} else {
						cachedSession = s.storer.DirectUpload()
					}
					if err != nil {
						logger.Debug("chunk upload stream: failed to create session", "error", err)
						logger.Error(nil, "chunk upload stream: failed to create session")
						sendErrorClose(websocket.CloseInternalServerErr, "session creation failed")
						return
					}
					sessionCache[batchIDHex] = cachedSession
					logger.Debug("chunk upload stream: cached new session", "batch_id", batchIDHex)
				} else {
					logger.Debug("chunk upload stream: reusing cached session", "batch_id", batchIDHex)
				}

				// Use the cached session directly - chunks arrive pre-stamped
				// No need for a stamper wrapper since client already signed the chunks
				chunkPutter = cachedSession

				// Use the chunk data without the stamp
				chunkData = potentialChunkData
			} else {
				// Stamp unmarshal failed - log for debugging
				logger.Debug("chunk upload stream: stamp unmarshal failed, treating message as unstamped chunk",
					"error", err,
					"message_size", len(msg),
					"stamp_size_expected", postage.StampSize,
					"potential_stamp_len", len(potentialStamp))
			}
			// If unmarshal failed, fall through to use the whole message as chunk data
		}

		// If we don't have a putter at this point, the client must provide per-chunk stamps
		if chunkPutter == nil {
			logger.Debug("chunk upload stream: no stamp provided")
			logger.Error(nil, "chunk upload stream: no batch ID in headers and no per-chunk stamp in message")
			sendErrorClose(websocket.CloseInternalServerErr, "batch ID or per-chunk stamp required")
			return
		}

		chunk, err = cac.NewWithDataSpan(chunkData)
		if err != nil {
			logger.Debug("chunk upload stream: create chunk failed", "error", err, "chunk_size", len(chunkData))
			logger.Error(nil, "chunk upload stream: create chunk failed")
			sendErrorClose(websocket.CloseInternalServerErr, "invalid chunk data")
			return
		}

		err = chunkPutter.Put(ctx, chunk)
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

		// Note: We don't call Done() here for per-chunk stamped uploads
		// because we're reusing the cached session across multiple chunks
		// The cached sessions are cleaned up in the defer function when the connection closes

		err = sendMsg(websocket.BinaryMessage, successWsMsg)
		if err != nil {
			s.logger.Debug("chunk upload stream: sending success message failed", "error", err)
			s.logger.Error(nil, "chunk upload stream: sending success message failed")
			return
		}
	}
}
