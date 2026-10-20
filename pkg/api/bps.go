// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
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
	bpsCloseEndOfStream    = 4004 // the owner ended the channel with an EOS

	bpsMaxCloseReason = 123   // websocket control frame payload limit minus the code
	bpsFrameHeader    = 1 + 8 // publish frame header: kind | index (big-endian)
	bpsMaxSOCSize     = swarm.HashSize + swarm.SocSignatureSize + swarm.SpanSize + swarm.ChunkSize
	bpsMaxFrameSize   = bpsFrameHeader + bpsMaxSOCSize
	// delivery frame header: kind | owner | challenge | index (big-endian)
	bpsDeliveryHeader = 1 + common.AddressLength + bps.ChallengeSize + 8
)

// bpsAuthTimeout bounds how long a publisher may take to send its AUTH. For the
// owner it is counted from before the join and is shorter than the broker's
// auth deadline, so that the broker never blocklists this node for a slow
// dApp; for a seat it is counted from the grant.
var bpsAuthTimeout = 25 * time.Second

var errBPSBrokerGone = errors.New("broker stream closed")

type bpsRequest struct {
	owner    common.Address
	topic    []byte
	broker   swarm.Address
	identity []byte
	cursor   uint64
}

// bpsGrantedMessage tells a seat's dApp that a roster naming its identity has
// reached the stream: it may now send its AUTH.
type bpsGrantedMessage struct {
	Type string `json:"type"`
}

type bpsChallengeMessage struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Broker      string `json:"broker"`
	DataTopic   string `json:"dataTopic"`
	AuthTopic   string `json:"authTopic"`
	RosterTopic string `json:"rosterTopic,omitempty"` // the owner's only
	EOSTopic    string `json:"eosTopic,omitempty"`    // the owner's only
}

// bpsDeliveryFrame encodes a delivery for the dApp: kind | owner | challenge |
// index | soc — what it needs to re-verify the chunk, whoever published it.
func bpsDeliveryFrame(msg bps.Message) []byte {
	data := make([]byte, 0, bpsDeliveryHeader+len(msg.SOC))
	data = append(data, byte(msg.Kind))
	data = append(data, msg.Owner...)
	data = append(data, msg.Challenge...)
	data = binary.BigEndian.AppendUint64(data, msg.Index)
	return append(data, msg.SOC...)
}

// bpsSessionEnded closes the socket for a session whose stream ended: with
// 4004 after an EOS, 4003 otherwise.
func (s *Service) bpsSessionEnded(conn *websocket.Conn, sess bps.Session) {
	if err := sess.Err(); errors.Is(err, bps.ErrEndOfStream) {
		s.bpsClose(conn, bpsCloseEndOfStream, "end of stream")
		return
	}
	s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(sess.Err()))
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
	// a stream declaring the owner is a pending publisher stream at the broker: it
	// would receive nothing and be blocklisted at the auth deadline
	if bytes.Equal(req.identity, req.owner.Bytes()) {
		jsonhttp.BadRequest(w, "identity is the owner: use publish")
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
	// the owner publishes by default; a seat names its own address
	if len(req.identity) == 0 {
		req.identity = req.owner.Bytes()
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
		e := "upgrade failed"
		logger.Debug(e, "error", err)
		logger.Error(nil, e)
		jsonhttp.InternalServerError(w, e)
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
		Broker:   req.broker,
		Spec:     bps.CohortSpec{Topic: req.topic, Principal: req.owner.Bytes()},
		Identity: req.identity,
		Position: bps.Position{Cursors: map[string]uint64{string(req.owner.Bytes()): req.cursor}},
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
				s.bpsSessionEnded(conn, sess)
				return
			}
			if err := s.bpsWrite(conn, websocket.BinaryMessage, bpsDeliveryFrame(msg)); err != nil {
				logger.Debug("write broadcast failed", "error", err)
				return
			}
		case <-frames:
			s.bpsClose(conn, bpsCloseInvalidMessage, "subscribe endpoint is read-only")
			return
		case <-sess.Done():
			s.bpsSessionEnded(conn, sess)
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

// bpsParseFrame checks a publish frame kind | index | soc the same way, and in
// the same order, as the broker will, or returns a close code and reason.
func bpsParseFrame(f bpsFrame, req bpsRequest, challenge []byte, authenticated bool) (bps.Kind, uint64, []byte, int, string) {
	if f.typ != websocket.BinaryMessage {
		return 0, 0, nil, bpsCloseInvalidMessage, "expected binary frame"
	}
	if len(f.data) < bpsFrameHeader {
		return 0, 0, nil, bpsCloseInvalidMessage, "short frame"
	}
	kind := bps.Kind(f.data[0])
	// a pending stream may send nothing but its AUTH; the broker would reset it
	if !authenticated && kind != bps.KindAuth {
		return 0, 0, nil, bpsCloseInvalidMessage, "expected auth"
	}
	owner := req.identity
	switch kind {
	case bps.KindData, bps.KindAuth:
	case bps.KindRoster, bps.KindEOS:
		// the service feeds are the owner's alone
		if !bytes.Equal(req.identity, req.owner.Bytes()) {
			return 0, 0, nil, bpsCloseInvalidMessage, "roster and eos are the owner's"
		}
		owner = req.owner.Bytes()
	default:
		return 0, 0, nil, bpsCloseInvalidMessage, "unknown kind"
	}
	index := binary.BigEndian.Uint64(f.data[1:bpsFrameHeader])
	chunk := f.data[bpsFrameHeader:]
	if err := bps.Verify(kind, challenge, index, chunk, req.topic, owner); err != nil {
		return 0, 0, nil, bpsCloseInvalidSOC, "invalid soc"
	}
	// the broker drops a roster that does not decode without a word
	if kind == bps.KindRoster {
		if _, err := bps.ParseRoster(chunk); err != nil {
			return 0, 0, nil, bpsCloseInvalidMessage, "invalid roster"
		}
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

	authTimer := time.NewTimer(bpsAuthTimeout)
	defer authTimer.Stop()

	// the publisher declares its identity — the owner's, or a seat the owner's
	// roster names — and authenticates with its AUTH: the owner at once, a
	// seat once a roster naming it has reached the stream, which upgrades it
	// in place
	owner := bytes.Equal(req.identity, req.owner.Bytes())
	sess, err := s.bps.Join(ctx, bps.JoinRequest{
		Broker:   req.broker,
		Spec:     bps.CohortSpec{Topic: req.topic, Principal: req.owner.Bytes()},
		Identity: req.identity,
	})
	if err != nil {
		logger.Debug("join failed", "broker", req.broker, "error", err)
		s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
		return
	}
	defer sess.Close()

	// a seat listens until it is granted; its auth deadline starts then
	granted := owner
	var grantedC <-chan struct{}
	if !owner {
		authTimer.Stop()
		grantedC = sess.Granted()
	}

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
	msg := bpsChallengeMessage{
		Type:      "challenge",
		Challenge: hex.EncodeToString(challenge),
		Broker:    req.broker.String(),
		DataTopic: hex.EncodeToString(dataTopic),
		AuthTopic: hex.EncodeToString(authTopic),
	}
	if owner {
		rosterTopic, err := bps.SessionTopic(bps.KindRoster, req.topic, challenge)
		if err != nil {
			s.bpsClose(conn, bpsCloseBrokerGone, "session topic")
			return
		}
		eosTopic, err := bps.SessionTopic(bps.KindEOS, req.topic, challenge)
		if err != nil {
			s.bpsClose(conn, bpsCloseBrokerGone, "session topic")
			return
		}
		msg.RosterTopic, msg.EOSTopic = hex.EncodeToString(rosterTopic), hex.EncodeToString(eosTopic)
	}
	if err := s.bpsWriteJSON(conn, msg); err != nil {
		logger.Debug("write challenge failed", "error", err)
		return
	}

	frames, gone := bpsReadFrames(conn, quit)
	authenticated := false

	for {
		select {
		case <-grantedC:
			grantedC, granted = nil, true
			authTimer.Reset(bpsAuthTimeout)
			if err := s.bpsWriteJSON(conn, bpsGrantedMessage{Type: "granted"}); err != nil {
				logger.Debug("write granted failed", "error", err)
				return
			}
		case f := <-frames:
			// a seat sends nothing before a roster names it
			if !granted {
				s.bpsClose(conn, bpsCloseInvalidMessage, "not granted")
				return
			}
			kind, index, chunk, code, reason := bpsParseFrame(f, req, challenge, authenticated)
			if code != 0 {
				s.bpsClose(conn, code, reason)
				return
			}
			if err := sess.Publish(ctx, kind, index, chunk); err != nil {
				s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
				return
			}
			if !authenticated {
				authenticated = true
				authTimer.Stop()
			}
		case msg, ok := <-sess.Messages():
			if !ok {
				s.bpsSessionEnded(conn, sess)
				return
			}
			if err := s.bpsWrite(conn, websocket.BinaryMessage, bpsDeliveryFrame(msg)); err != nil {
				logger.Debug("write broadcast failed", "error", err)
				return
			}
		case <-authTimer.C:
			if !authenticated {
				s.bpsClose(conn, bpsCloseInvalidMessage, "auth timeout")
				return
			}
		case <-sess.Done():
			s.bpsSessionEnded(conn, sess)
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
