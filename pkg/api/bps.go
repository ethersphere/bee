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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	bpsCloseInvalidSOC     = 4001 // chunk failed verification
	bpsCloseInvalidMessage = 4002 // malformed or disallowed frame
	bpsCloseBrokerGone     = 4003 // the stream to the broker ended
	bpsCloseEndOfStream    = 4004 // the principal sent EOS
	bpsCloseAuthTimeout    = 4005 // the principal's AUTH came too late

	bpsMaxCloseReason = 123   // websocket control frame limit minus the code
	bpsFrameHeader    = 1 + 8 // publish frame header: kind | index (big-endian)
	bpsMaxFrameSize   = bpsFrameHeader + swarm.SocMaxChunkSize
	// delivery frame header: kind | owner | challenge | index (big-endian)
	bpsDeliveryHeader = 1 + common.AddressLength + bps.ChallengeSize + 8

	// bpsGranted is the text frame telling the client it may send its AUTH.
	bpsGranted = "granted"
)

// bpsAuthTimeout is how long the principal has to send its AUTH. It starts
// before the join and is shorter than the broker's timeout, so the broker never blocklists this node
// because of a slow client.
var bpsAuthTimeout = 25 * time.Second

// errBPSInvalidRoster means the roster query parameter does not parse.
var errBPSInvalidRoster = errors.New("invalid roster")

// Reasons a socket closes, besides the session's own errors. bpsCloseCode maps
// them to close codes.
var (
	errBPSInvalidMessage = errors.New("invalid message")
	errBPSAuthTimeout    = errors.New("auth timeout")
	errBPSBrokerGone     = errors.New("broker stream closed")
	errBPSClientGone     = errors.New("client gone")
	errBPSShutdown       = errors.New("node shutting down")
)

type bpsRequest struct {
	principal common.Address
	topic     []byte
	broker    swarm.Address
	owner     []byte       // signer address, empty to only listen
	position  bps.Position // from the cursor, roster and rosterCursor query
}

type bpsChallengeMsg struct {
	Challenge   string `json:"challenge"`
	Broker      string `json:"broker"`
	DataTopic   string `json:"dataTopic"`
	AuthTopic   string `json:"authTopic"`
	RosterTopic string `json:"rosterTopic,omitempty"` // principal only
	EOSTopic    string `json:"eosTopic,omitempty"`    // principal only
}

// newBPSChallengeMsg returns the challenge and the feed topics for each kind
// the owner may publish. The client needs them to build its chunks.
func newBPSChallengeMsg(req bpsRequest, challenge []byte, isPrincipal bool) (bpsChallengeMsg, error) {
	msg := bpsChallengeMsg{
		Challenge: hex.EncodeToString(challenge),
		Broker:    req.broker.String(),
	}
	type sessionTopic struct {
		kind bps.Kind
		dst  *string
	}
	topics := []sessionTopic{{bps.KindData, &msg.DataTopic}, {bps.KindAuth, &msg.AuthTopic}}
	if isPrincipal {
		topics = append(topics, sessionTopic{bps.KindRoster, &msg.RosterTopic}, sessionTopic{bps.KindEOS, &msg.EOSTopic})
	}
	for _, t := range topics {
		topic, err := bps.SessionTopic(t.kind, req.topic, challenge)
		if err != nil {
			return bpsChallengeMsg{}, fmt.Errorf("session topic: %w", err)
		}
		*t.dst = hex.EncodeToString(topic)
	}
	return msg, nil
}

// bpsDeliveryFrame encodes a delivery as kind | owner | challenge | index | soc.
// That is enough for the client to verify the chunk itself.
func bpsDeliveryFrame(msg bps.Message) []byte {
	data := make([]byte, 0, bpsDeliveryHeader+len(msg.SOC))
	data = append(data, byte(msg.Kind))
	data = append(data, msg.Owner...)
	data = append(data, msg.Challenge...)
	data = binary.BigEndian.AppendUint64(data, msg.Index)
	return append(data, msg.SOC...)
}

func (s *Service) bpsWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("bps").Build()
	req, ok := s.bpsParseRequest(w, r, logger)
	if !ok {
		return
	}
	conn, ok := s.bpsUpgrade(w, r, logger)
	if !ok {
		return
	}
	s.wsWg.Add(1)
	go s.bpsRun(conn, req, logger)
}

// bpsParseRequest validates the request before the websocket upgrade.
// It writes the HTTP error response itself and reports whether to continue.
func (s *Service) bpsParseRequest(w http.ResponseWriter, r *http.Request, logger log.Logger) (bpsRequest, bool) {
	if s.bps == nil {
		jsonhttp.ServiceUnavailable(w, "bps unavailable")
		return bpsRequest{}, false
	}

	paths := struct {
		Principal common.Address `map:"principal" validate:"required"`
		Topic     []byte         `map:"topic" validate:"required,len=32"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return bpsRequest{}, false
	}

	queries := struct {
		Broker       swarm.Address `map:"broker"`
		Owner        []byte        `map:"owner" validate:"omitempty,len=20"`
		Cursor       uint64        `map:"cursor"`
		Roster       string        `map:"roster"`
		RosterCursor uint64        `map:"rosterCursor"`
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

	pos, err := bpsPosition(paths.Principal.Bytes(), queries.Cursor, queries.Roster, queries.RosterCursor)
	if err != nil {
		jsonhttp.BadRequest(w, err.Error())
		return bpsRequest{}, false
	}

	return bpsRequest{
		principal: paths.Principal,
		topic:     paths.Topic,
		broker:    queries.Broker,
		owner:     queries.Owner,
		position:  pos,
	}, true
}

// bpsPosition builds a Position from the query. cursor is the principal's next
// index. roster is a comma-separated list of address:index pairs, one per
// publisher. rosterCursor is the next roster index. A client tracks these from
// its deliveries and passes them back when it reconnects.
func bpsPosition(principal []byte, cursor uint64, roster string, rosterCursor uint64) (bps.Position, error) {
	pos := bps.Position{
		Cursors:      map[string]uint64{string(principal): cursor},
		RosterCursor: rosterCursor,
	}
	if roster == "" {
		return pos, nil
	}
	entries := strings.Split(roster, ",")
	if len(entries) > bps.MaxRosterSize {
		return pos, fmt.Errorf("%w: more than %d addresses", errBPSInvalidRoster, bps.MaxRosterSize)
	}
	for _, e := range entries {
		addr, index, ok := strings.Cut(e, ":")
		if !ok {
			return pos, fmt.Errorf("%w: %q is not address:index", errBPSInvalidRoster, e)
		}
		a, err := hex.DecodeString(strings.TrimPrefix(addr, "0x"))
		if err != nil || len(a) != common.AddressLength {
			return pos, fmt.Errorf("%w: address %q", errBPSInvalidRoster, addr)
		}
		if bytes.Equal(a, principal) {
			return pos, fmt.Errorf("%w: the principal must not be on the roster", errBPSInvalidRoster)
		}
		n, err := strconv.ParseUint(index, 10, 64)
		if err != nil {
			return pos, fmt.Errorf("%w: index %q", errBPSInvalidRoster, index)
		}
		if _, dup := pos.Cursors[string(a)]; dup {
			return pos, fmt.Errorf("%w: address %q twice", errBPSInvalidRoster, addr)
		}
		pos.Cursors[string(a)] = n
		pos.Roster = append(pos.Roster, a)
	}
	return pos, nil
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

// bpsCloseCode returns the websocket close code for err.
func bpsCloseCode(err error) int {
	switch {
	case errors.Is(err, bps.ErrEndOfStream):
		return bpsCloseEndOfStream
	case errors.Is(err, bps.ErrInvalidSOC):
		return bpsCloseInvalidSOC
	case errors.Is(err, errBPSInvalidMessage),
		errors.Is(err, bps.ErrInvalidMessage),
		errors.Is(err, bps.ErrInvalidRoster),
		errors.Is(err, bps.ErrNotPublisher):
		return bpsCloseInvalidMessage
	case errors.Is(err, errBPSAuthTimeout):
		return bpsCloseAuthTimeout
	case errors.Is(err, errBPSShutdown):
		return websocket.CloseGoingAway
	default:
		return bpsCloseBrokerGone
	}
}

// bpsClose sends a close frame for err. It may run while the inbox writes.
// Errors are ignored because the connection is closed right after.
func bpsClose(conn *websocket.Conn, err error) {
	reason := err.Error()
	if len(reason) > bpsMaxCloseReason {
		reason = strings.ToValidUTF8(reason[:bpsMaxCloseReason], "")
	}
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(bpsCloseCode(err), reason), time.Now().Add(writeDeadline))
}

// bpsParseFrame splits a client frame into kind, index and soc. The session
// checks the rest.
func bpsParseFrame(typ int, data []byte) (bps.Kind, uint64, []byte, error) {
	if typ != websocket.BinaryMessage {
		return 0, 0, nil, fmt.Errorf("%w: expected a binary frame", errBPSInvalidMessage)
	}
	if len(data) < bpsFrameHeader {
		return 0, 0, nil, fmt.Errorf("%w: short frame", errBPSInvalidMessage)
	}
	return bps.Kind(data[0]), binary.BigEndian.Uint64(data[1:bpsFrameHeader]), data[bpsFrameHeader:], nil
}

// bpsRun connects a client's websocket to a session with the broker. The
// principal must send its AUTH within bpsAuthTimeout. Another owner may send
// its AUTH once it gets the "granted" frame. Without an owner the client only
// listens and gets no challenge. The first of outbox, inbox or node shutdown to
// stop decides how the socket closes.
func (s *Service) bpsRun(conn *websocket.Conn, req bpsRequest, logger log.Logger) {
	defer s.wsWg.Done()
	defer func() { _ = conn.Close() }()

	ctx, stop := context.WithCancelCause(context.Background())
	defer stop(nil)
	go func() {
		select {
		case <-s.quit:
			stop(errBPSShutdown)
		case <-ctx.Done():
		}
	}()

	isPrincipal := bytes.Equal(req.owner, req.principal.Bytes())
	authTimer := time.NewTimer(bpsAuthTimeout)
	defer authTimer.Stop()
	if !isPrincipal {
		authTimer.Stop()
	}

	sess, err := s.bps.Join(ctx, bps.JoinRequest{
		Broker:   req.broker,
		Spec:     bps.CohortSpec{Topic: req.topic, Principal: req.principal.Bytes()},
		Owner:    req.owner,
		Position: req.position,
	})
	if err != nil {
		logger.Debug("join failed", "broker", req.broker, "error", err)
		stop(fmt.Errorf("%w: %w", errBPSBrokerGone, err))
		bpsClose(conn, context.Cause(ctx))
		return
	}
	defer sess.Close()

	// a listener signs nothing, so it needs no challenge
	if len(req.owner) > 0 {
		msg, err := newBPSChallengeMsg(req, sess.Challenge(), isPrincipal)
		if err != nil {
			logger.Debug("challenge message failed", "error", err)
			bpsClose(conn, fmt.Errorf("%w: %w", errBPSBrokerGone, err))
			return
		}
		if err := s.bpsWriteJSON(conn, msg); err != nil {
			logger.Debug("ws write failed", "error", err)
			return
		}
	}

	var wg sync.WaitGroup
	wg.Go(func() { stop(bpsHandleOutbox(ctx, conn, sess, authTimer)) })
	wg.Go(func() { stop(s.bpsHandleInbox(ctx, conn, sess, isPrincipal, authTimer.C, logger)) })

	<-ctx.Done()
	if err := context.Cause(ctx); !errors.Is(err, errBPSClientGone) {
		bpsClose(conn, err)
	}
	// unblock the outbox's read
	_ = conn.Close()
	wg.Wait()
}

// bpsHandleOutbox publishes the client's frames until a read fails or the
// session rejects a frame. It runs even before the stream may publish, because
// only a read notices a client that has gone. The session rejects early frames.
func bpsHandleOutbox(ctx context.Context, conn *websocket.Conn, sess bps.Session, authTimer *time.Timer) error {
	for {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			return errBPSClientGone
		}
		kind, index, chunk, err := bpsParseFrame(typ, data)
		if err != nil {
			return err
		}
		if err := sess.Publish(ctx, kind, index, chunk); err != nil {
			return err
		}
		if kind == bps.KindAuth {
			authTimer.Stop()
		}
	}
}

// bpsHandleInbox writes deliveries, the "granted" frame and pings to the
// client. It stops when the session or socket ends or the principal's AUTH is
// late. Apart from the close frame, it is the only writer to the socket.
func (s *Service) bpsHandleInbox(ctx context.Context, conn *websocket.Conn, sess bps.Session, isPrincipal bool, authC <-chan time.Time, logger log.Logger) error {
	ticker := time.NewTicker(s.WsPingPeriod)
	defer ticker.Stop()

	// the principal needs no "granted" frame
	var granted <-chan struct{}
	if !isPrincipal {
		granted = sess.Granted()
	}

	for {
		var err error
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sess.Done():
			if err := sess.Err(); err != nil {
				return err
			}
			return errBPSBrokerGone
		case <-authC:
			return errBPSAuthTimeout
		case <-granted:
			granted = nil
			err = s.bpsWrite(conn, websocket.TextMessage, []byte(bpsGranted))
		case msg := <-sess.Messages():
			err = s.bpsWrite(conn, websocket.BinaryMessage, bpsDeliveryFrame(msg))
		case <-ticker.C:
			err = s.bpsWrite(conn, websocket.PingMessage, nil)
		}
		if err != nil {
			logger.Debug("ws write failed", "error", err)
			return errBPSClientGone
		}
	}
}
