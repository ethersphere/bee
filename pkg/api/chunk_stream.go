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
		gone   = make(chan struct{})
		ticker = time.NewTicker(time.Second * 4)
		err    error
	)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debugf("chunk stream handler: client gone. code %d message %s", code, text)
		close(gone)
		return nil
	})

	// default handlers for ping/pong
	conn.SetPingHandler(nil)
	conn.SetPongHandler(nil)

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

	sendErrorAndClose := func(errmsg string) error {
		return sendMsg(websocket.CloseMessage, []byte(errmsg))
	}

	for {
		select {
		case <-s.quit:
			// shutdown
			err = sendErrorAndClose("node shutting down")
			if err != nil {
				s.logger.Debugf("failed sending close message: %v", err)
			}
			return
		case <-gone:
			// client gone
			return
		case <-ticker.C:
			err = sendMsg(websocket.PingMessage, nil)
			if err != nil {
				s.logger.Debugf("failed sending ping message: %v", err)
				return
			}
		default:
			// if there is no indication to stop, go ahead and read the next message
		}

		err = conn.SetReadDeadline(time.Now().Add(readDeadline))
		if err != nil {
			s.logger.Debugf("chunk stream set read deadline: %v", err)
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			s.logger.Debugf("chunk stream handler read message error: %v", err)
			return
		}

		if mt != websocket.BinaryMessage {
			s.logger.Debug("unexpected message received from client", mt)
			sendErrorAndClose("invalid message")
			return
		}

		if len(msg) < swarm.SpanSize {
			s.logger.Debug("chunk upload: not enough data")
			return
		}

		chunk, err := cac.NewWithDataSpan(msg)
		if err != nil {
			s.logger.Debugf("chunk upload: create chunk error: %v", err)
			return
		}

		seen, err := putter.Put(ctx, mode, chunk)
		if err != nil {
			s.logger.Debugf("chunk upload: chunk write error: %v, addr %s", err, chunk.Address())
			switch {
			case errors.Is(err, postage.ErrBucketFull):
				sendErrorAndClose("batch is overissued")
			default:
				sendErrorAndClose("chunk write error")
			}
			return
		} else if len(seen) > 0 && seen[0] && tag != nil {
			err := tag.Inc(tags.StateSeen)
			if err != nil {
				s.logger.Debugf("chunk upload: increment tag", err)
				sendErrorAndClose("failed incrementing tag")
				return
			}
		}

		if tag != nil {
			// indicate that the chunk is stored
			err = tag.Inc(tags.StateStored)
			if err != nil {
				s.logger.Debugf("chunk upload: increment tag", err)
				sendErrorAndClose("failed incrementing tag")
				return
			}
		}

		if pin {
			if err := s.pinning.CreatePin(ctx, chunk.Address(), false); err != nil {
				s.logger.Debugf("chunk upload: creation of pin for %q failed: %v", chunk.Address(), err)
				sendErrorAndClose("failed creating pin")
				return
			}
		}

		err = sendMsg(websocket.TextMessage, []byte("success"))
		if err != nil {
			s.logger.Debugf("failed sending success msg: %v", err)
			return
		}
	}
}
