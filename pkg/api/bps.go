// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

const (
	bpsCloseInvalidClaim   = 4001 // claim failed local verification or was malformed
	bpsCloseInvalidMessage = 4002 // frame violates the endpoint protocol
	bpsCloseBrokerGone     = 4003 // the p2p stream to the broker ended

	bpsMaxCloseReason = 123 // websocket control frame payload limit minus the code
	bpsMaxFrameSize   = swarm.HashSize + swarm.SocSignatureSize + swarm.SpanSize + swarm.ChunkSize
)

// bpsTopicPrefix domain-separates the cohort SOC id from the owner's other SOCs.
var bpsTopicPrefix = []byte("bps-claim")

// bpsClaimTimeout bounds how long a publisher may take to send its claim.
var bpsClaimTimeout = 30 * time.Second

var errBPSBrokerGone = errors.New("broker stream closed")

// BPSService joins cohorts at a broker.
type BPSService interface {
	Join(ctx context.Context, broker, addr swarm.Address) (BPSSession, error)
}

// BPSSession is a single joined p2p stream to a broker.
type BPSSession interface {
	// Challenge is the nonce the broker issued for this stream.
	Challenge() []byte
	// Messages yields raw SOC bytes broadcast by the broker.
	// Implementations must not block when Messages is not drained (the
	// publish endpoint never reads it). Closing the channel is treated
	// as the stream ending.
	Messages() <-chan []byte
	// Claim writes the claim SOC to the broker. The broker does not reply.
	Claim(ctx context.Context, soc []byte) error
	// Publish writes a broadcast SOC to the broker.
	Publish(ctx context.Context, soc []byte) error
	// Done is closed when the p2p stream ends.
	Done() <-chan struct{}
	// Err reports why the stream ended. Valid after Done is closed.
	Err() error
	Close() error
}

type bpsRequest struct {
	owner  common.Address
	id     []byte
	addr   swarm.Address
	broker swarm.Address
}

type bpsChallengeMessage struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Broker    string `json:"broker"`
	ID        string `json:"id"`
}

type bpsClaimMessage struct {
	Type      string `json:"type"`
	Signature string `json:"signature"`
}

type bpsClaimSentMessage struct {
	Type string `json:"type"`
}

// bpsTopicAddress derives the cohort SOC id and address for owner and topic.
func bpsTopicAddress(owner common.Address, topic []byte) ([]byte, swarm.Address, error) {
	id, err := crypto.LegacyKeccak256(append(append([]byte{}, bpsTopicPrefix...), topic...))
	if err != nil {
		return nil, swarm.ZeroAddress, err
	}
	addr, err := soc.CreateAddress(id, owner.Bytes())
	if err != nil {
		return nil, swarm.ZeroAddress, err
	}
	return id, addr, nil
}

func (s *Service) bpsSubscribeWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("bps_subscribe").Build()
	req, ok := s.bpsParseRequest(w, r, logger)
	if !ok {
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
		Broker swarm.Address `map:"broker"`
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

	id, addr, err := bpsTopicAddress(paths.Owner, paths.Topic)
	if err != nil {
		logger.Debug("derive topic address failed", "error", err)
		jsonhttp.InternalServerError(w, "derive topic address failed")
		return bpsRequest{}, false
	}
	return bpsRequest{owner: paths.Owner, id: id, addr: addr, broker: queries.Broker}, true
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

	sess, err := s.bps.Join(ctx, req.broker, req.addr)
	if err != nil {
		logger.Debug("join failed", "broker", req.broker, "error", err)
		s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
		return
	}
	defer sess.Close()

	frames, gone := bpsReadFrames(conn, quit)

	for {
		select {
		case data, ok := <-sess.Messages():
			if !ok {
				s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(sess.Err()))
				return
			}
			if !soc.Valid(swarm.NewChunk(req.addr, data)) {
				logger.Debug("dropping invalid broadcast", "address", req.addr)
				continue
			}
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

// bpsVerifyClaim checks a claim frame the same way the broker will and
// returns the claim SOC bytes, or a close code and reason on failure.
func bpsVerifyClaim(f bpsFrame, req bpsRequest, challenge []byte) ([]byte, int, string) {
	if f.typ != websocket.TextMessage {
		return nil, bpsCloseInvalidMessage, "expected claim"
	}
	var m bpsClaimMessage
	if err := json.Unmarshal(f.data, &m); err != nil {
		return nil, bpsCloseInvalidClaim, "malformed claim"
	}
	if m.Type != "claim" {
		return nil, bpsCloseInvalidMessage, "expected claim"
	}
	sig, err := hex.DecodeString(m.Signature)
	if err != nil || len(sig) != swarm.SocSignatureSize {
		return nil, bpsCloseInvalidClaim, "invalid signature"
	}
	payload := make([]byte, 0, len(challenge)+swarm.HashSize)
	payload = append(append(payload, challenge...), req.broker.Bytes()...)
	ch, err := cac.New(payload)
	if err != nil {
		return nil, bpsCloseInvalidClaim, "invalid claim payload"
	}
	sc, err := soc.NewSigned(req.id, ch, req.owner.Bytes(), sig)
	if err != nil {
		return nil, bpsCloseInvalidClaim, "invalid claim"
	}
	chunk, err := sc.Chunk()
	if err != nil {
		return nil, bpsCloseInvalidClaim, "invalid claim"
	}
	// NewSigned does not verify the signature; Valid recovers the signer
	// and checks that it maps to the cohort address.
	if !chunk.Address().Equal(req.addr) || !soc.Valid(chunk) {
		return nil, bpsCloseInvalidClaim, "claim verification failed"
	}
	return chunk.Data(), 0, ""
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

	sess, err := s.bps.Join(ctx, req.broker, req.addr)
	if err != nil {
		logger.Debug("join failed", "broker", req.broker, "error", err)
		s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
		return
	}
	defer sess.Close()

	challenge := sess.Challenge()
	if len(challenge) != swarm.HashSize {
		logger.Debug("invalid challenge length", "broker", req.broker, "length", len(challenge))
		s.bpsClose(conn, bpsCloseBrokerGone, "invalid challenge")
		return
	}
	if err := s.bpsWriteJSON(conn, bpsChallengeMessage{
		Type:      "challenge",
		Challenge: hex.EncodeToString(challenge),
		Broker:    req.broker.String(),
		ID:        hex.EncodeToString(req.id),
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
			if !claimed {
				claim, code, reason := bpsVerifyClaim(f, req, challenge)
				if code != 0 {
					s.bpsClose(conn, code, reason)
					return
				}
				if err := sess.Claim(ctx, claim); err != nil {
					s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
					return
				}
				claimed = true
				claimTimer.Stop()
				if err := s.bpsWriteJSON(conn, bpsClaimSentMessage{Type: "claim_sent"}); err != nil {
					logger.Debug("write claim_sent failed", "error", err)
					return
				}
				continue
			}
			if f.typ != websocket.BinaryMessage {
				s.bpsClose(conn, bpsCloseInvalidMessage, "expected binary soc")
				return
			}
			if !soc.Valid(swarm.NewChunk(req.addr, f.data)) {
				s.bpsClose(conn, bpsCloseInvalidMessage, "invalid soc")
				return
			}
			if err := sess.Publish(ctx, f.data); err != nil {
				s.bpsClose(conn, bpsCloseBrokerGone, bpsErrReason(err))
				return
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
