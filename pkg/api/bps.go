// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

const (
	bpsCloseInvalidSOC     = 4001 // a chunk that does not validate under the session challenge
	bpsCloseInvalidMessage = 4002 // frame violates the endpoint protocol
	bpsCloseBrokerGone     = 4003 // the p2p stream to the broker ended

	bpsMaxCloseReason = 123   // websocket control frame payload limit minus the code
	bpsFrameHeader    = 1 + 8 // publish frame header: kind | index (big-endian)
	bpsMaxSOCSize     = swarm.HashSize + swarm.SocSignatureSize + swarm.SpanSize + swarm.ChunkSize
	bpsMaxFrameSize   = bpsFrameHeader + bpsMaxSOCSize
)

// bpsClaimTimeout bounds how long a publisher may take to send its first frame.
var bpsClaimTimeout = 30 * time.Second

var errBPSBrokerGone = errors.New("broker stream closed")

type bpsRequest struct {
	owner    common.Address
	topic    []byte
	broker   swarm.Address
	identity []byte
	cursor   uint64
}

type bpsChallengeMessage struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Broker    string `json:"broker"`
	DataTopic string `json:"dataTopic"`
	AuthTopic string `json:"authTopic"`
}

func (s *Service) bpsSubscribeWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("bps_subscribe").Build()
	req, ok := s.bpsParseRequest(w, r, logger)
	if !ok {
		return
	}
	if len(req.identity) == 0 {
		jsonhttp.BadRequest(w, "missing identity")
		return
	}
	conn, ok := s.bpsUpgrade(w, r, logger)
	if !ok {
		return
	}
	s.wsWg.Add(1)
	go s.bpsSubscribeWs(conn, req, logger)
}

func (s *Service) bpsPublishWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("bps_publish").Build()
	req, ok := s.bpsParseRequest(w, r, logger)
	if !ok {
		return
	}
	conn, ok := s.bpsUpgrade(w, r, logger)
	if !ok {
		return
	}
	s.wsWg.Add(1)
	go s.bpsPublishWs(conn, req, logger)
}

// bpsParseRequest validates the request before the websocket upgrade.
// It writes the HTTP error response itself and reports whether to continue.
func (s *Service) bpsParseRequest(w http.ResponseWriter, r *http.Request, logger log.Logger) (bpsRequest, bool) {
	if s.bps == nil {
		jsonhttp.ServiceUnavailable(w, "bps unavailable")
		return bpsRequest{}, false
	}

	paths := struct {
		Owner common.Address `map:"owner" validate:"required"`
		Topic []byte         `map:"topic" validate:"required,len=32"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return bpsRequest{}, false
	}

	queries := struct {
		Broker   swarm.Address `map:"broker"`
		Identity []byte        `map:"identity" validate:"omitempty,len=20"`
		Cursor   uint64        `map:"cursor"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return bpsRequest{}, false
	}
	if queries.Broker.IsZero() {
		jsonhttp.BadRequest(w, "missing broker")
		return bpsRequest{}, false
	}
	if len(queries.Broker.Bytes()) != swarm.HashSize {
		jsonhttp.BadRequest(w, "invalid broker")
		return bpsRequest{}, false
	}
	if s.overlay != nil && queries.Broker.Equal(*s.overlay) {
		jsonhttp.BadRequest(w, "broker cannot be this node")
		return bpsRequest{}, false
	}

	return bpsRequest{
		owner:    paths.Owner,
		topic:    paths.Topic,
		broker:   queries.Broker,
		identity: queries.Identity,
		cursor:   queries.Cursor,
	}, true
}

func (s *Service) bpsUpgrade(w http.ResponseWriter, r *http.Request, logger log.Logger) (*websocket.Conn, bool) {
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
		return nil, false
	}
	conn.SetReadLimit(bpsMaxFrameSize)
	return conn, true
}

func (s *Service) bpsWrite(conn *websocket.Conn, messageType int, data []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, data)
}

func (s *Service) bpsWriteJSON(conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.bpsWrite(conn, websocket.TextMessage, b)
}

// bpsClose sends a close frame with code and reason. Errors are ignored
// since the connection is torn down right after.
func (s *Service) bpsClose(conn *websocket.Conn, code int, reason string) {
	if len(reason) > bpsMaxCloseReason {
		reason = strings.ToValidUTF8(reason[:bpsMaxCloseReason], "")
	}
	_ = s.bpsWrite(conn, websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
}

func bpsErrReason(err error) string {
	if err == nil {
		return errBPSBrokerGone.Error()
	}
	return err.Error()
}

type bpsFrame struct {
	typ  int
	data []byte
}

// bpsReadFrames reads client frames until the connection errors or quit is
// closed. The returned gone channel is closed when reading stops.
func bpsReadFrames(conn *websocket.Conn, quit <-chan struct{}) (<-chan bpsFrame, <-chan struct{}) {
	frames := make(chan bpsFrame)
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		for {
			typ, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			select {
			case frames <- bpsFrame{typ: typ, data: data}:
			case <-quit:
				return
			}
		}
	}()
	return frames, gone
}

func (s *Service) bpsSubscribeWs(conn *websocket.Conn, req bpsRequest, logger log.Logger) {
	defer s.wsWg.Done()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-s.quit:
			cancel()
		case <-ctx.Done():
		}
	}()
	quit := make(chan struct{})
	ticker := time.NewTicker(s.WsPingPeriod)
	defer func() {
		ticker.Stop()
		close(quit)
		cancel()
		_ = conn.Close()
	}()

	sess, err := s.bps.Join(ctx, bps.JoinRequest{
		Broker: req.broker,
		Topic:  req.topic,
		Admin:  req.owner.Bytes(),
		Addr:   req.identity,
		Cursor: req.cursor,
	})
	if err != nil {
		logger.Debug("join failed", "broker", req.broker, "error", err)
		s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
		return
	}
	defer sess.Close()

	frames, gone := bpsReadFrames(conn, quit)

	for {
		select {
		case msg, ok := <-sess.Messages():
			if !ok {
				s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(sess.Err()))
				return
			}
			// challenge | index | soc: what the client needs to re-verify the chunk
			data := make([]byte, 0, bps.ChallengeSize+8+len(msg.SOC))
			data = append(data, msg.Challenge...)
			data = binary.BigEndian.AppendUint64(data, msg.Index)
			data = append(data, msg.SOC...)
			if err := s.bpsWrite(conn, websocket.BinaryMessage, data); err != nil {
				logger.Debug("write broadcast failed", "error", err)
				return
			}
		case <-frames:
			s.bpsClose(conn, bpsCloseInvalidMessage, "subscribe endpoint is read-only")
			return
		case <-sess.Done():
			s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(sess.Err()))
			return
		case <-gone:
			return
		case <-s.quit:
			_ = s.bpsWrite(conn, websocket.CloseMessage, []byte{})
			return
		case <-ticker.C:
			if err := s.bpsWrite(conn, websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// bpsParseFrame checks a publish frame kind | index | soc the same way the
// broker will, or returns a close code and reason on failure.
func bpsParseFrame(f bpsFrame, req bpsRequest, challenge []byte) (bps.Kind, uint64, []byte, int, string) {
	if f.typ != websocket.BinaryMessage {
		return 0, 0, nil, bpsCloseInvalidMessage, "expected binary frame"
	}
	if len(f.data) < bpsFrameHeader {
		return 0, 0, nil, bpsCloseInvalidMessage, "short frame"
	}
	kind := bps.Kind(f.data[0])
	if kind != bps.KindData && kind != bps.KindAuth {
		return 0, 0, nil, bpsCloseInvalidMessage, "unknown kind"
	}
	index := binary.BigEndian.Uint64(f.data[1:bpsFrameHeader])
	chunk := f.data[bpsFrameHeader:]
	if err := bps.Verify(kind, challenge, index, chunk, req.topic, req.owner.Bytes()); err != nil {
		return 0, 0, nil, bpsCloseInvalidSOC, "invalid soc"
	}
	return kind, index, chunk, 0, ""
}

func (s *Service) bpsPublishWs(conn *websocket.Conn, req bpsRequest, logger log.Logger) {
	defer s.wsWg.Done()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-s.quit:
			cancel()
		case <-ctx.Done():
		}
	}()
	quit := make(chan struct{})
	ticker := time.NewTicker(s.WsPingPeriod)
	defer func() {
		ticker.Stop()
		close(quit)
		cancel()
		_ = conn.Close()
	}()

	// the publisher declares the admin's address and claims with its first frame
	sess, err := s.bps.Join(ctx, bps.JoinRequest{
		Broker: req.broker,
		Topic:  req.topic,
		Admin:  req.owner.Bytes(),
		Addr:   req.owner.Bytes(),
	})
	if err != nil {
		logger.Debug("join failed", "broker", req.broker, "error", err)
		s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
		return
	}
	defer sess.Close()

	challenge := sess.Challenge()
	if len(challenge) != bps.ChallengeSize {
		logger.Debug("invalid challenge length", "broker", req.broker, "length", len(challenge))
		s.bpsClose(conn, bpsCloseBrokerGone, "invalid challenge")
		return
	}
	dataTopic, err := bps.SessionTopic(bps.KindData, req.topic, challenge)
	if err != nil {
		s.bpsClose(conn, bpsCloseBrokerGone, "session topic")
		return
	}
	authTopic, err := bps.SessionTopic(bps.KindAuth, req.topic, challenge)
	if err != nil {
		s.bpsClose(conn, bpsCloseBrokerGone, "session topic")
		return
	}
	if err := s.bpsWriteJSON(conn, bpsChallengeMessage{
		Type:      "challenge",
		Challenge: hex.EncodeToString(challenge),
		Broker:    req.broker.String(),
		DataTopic: hex.EncodeToString(dataTopic),
		AuthTopic: hex.EncodeToString(authTopic),
	}); err != nil {
		logger.Debug("write challenge failed", "error", err)
		return
	}

	claimTimer := time.NewTimer(bpsClaimTimeout)
	defer claimTimer.Stop()

	frames, gone := bpsReadFrames(conn, quit)
	claimed := false

	for {
		select {
		case f := <-frames:
			kind, index, chunk, code, reason := bpsParseFrame(f, req, challenge)
			if code != 0 {
				s.bpsClose(conn, code, reason)
				return
			}
			if err := sess.Publish(ctx, kind, index, chunk); err != nil {
				s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
				return
			}
			if !claimed {
				claimed = true
				claimTimer.Stop()
			}
		case <-claimTimer.C:
			if !claimed {
				s.bpsClose(conn, bpsCloseInvalidMessage, "claim timeout")
				return
			}
		case <-sess.Done():
			s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(sess.Err()))
			return
		case <-gone:
			return
		case <-s.quit:
			_ = s.bpsWrite(conn, websocket.CloseMessage, []byte{})
			return
		case <-ticker.C:
			if err := s.bpsWrite(conn, websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
