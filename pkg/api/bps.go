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
	bpsCloseInvalidSOC     = 4001 // a chunk that does not validate under the session challenge
	bpsCloseInvalidMessage = 4002 // a frame violates the endpoint protocol
	bpsCloseBrokerGone     = 4003 // the p2p stream to the broker ended
	bpsCloseEndOfStream    = 4004 // the owner ended the channel with an EOS
	bpsCloseAuthTimeout    = 4005 // the owner's AUTH did not arrive in time

	bpsMaxCloseReason = 123   // websocket control frame payload limit minus the code
	bpsFrameHeader    = 1 + 8 // publish frame header: kind | index (big-endian)
	bpsMaxFrameSize   = bpsFrameHeader + swarm.SocMaxChunkSize
	// delivery frame header: kind | owner | challenge | index (big-endian)
	bpsDeliveryHeader = 1 + common.AddressLength + bps.ChallengeSize + 8

	// bpsGranted is the text frame that tells a stream a roster naming its
	// identity has reached it: it may now send its AUTH.
	bpsGranted = "granted"
)

// bpsAuthTimeout bounds how long the owner may take to send its AUTH. It is
// counted from before the join and is shorter than the broker's auth deadline,
// so that the broker never blocklists this node for a slow dApp.
var bpsAuthTimeout = 25 * time.Second

// errBPSInvalidRoster is a roster query parameter that does not parse.
var errBPSInvalidRoster = errors.New("invalid roster")

// The reasons a socket ends that are the bridge's own; bpsCloseCode maps them,
// and the session's, to close codes.
var (
	errBPSInvalidMessage = errors.New("invalid message")
	errBPSAuthTimeout    = errors.New("auth timeout")
	errBPSBrokerGone     = errors.New("broker stream closed")
	errBPSClientGone     = errors.New("client gone")
	errBPSShutdown       = errors.New("node shutting down")
)

type bpsRequest struct {
	owner    common.Address
	topic    []byte
	broker   swarm.Address
	identity []byte
	position bps.Position // the place on the channel: cursor, roster, rosterCursor
}

type bpsChallengeMsg struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Broker      string `json:"broker"`
	DataTopic   string `json:"dataTopic"`
	AuthTopic   string `json:"authTopic"`
	RosterTopic string `json:"rosterTopic,omitempty"` // the owner's only
	EOSTopic    string `json:"eosTopic,omitempty"`    // the owner's only
}

// newBPSChallengeMsg returns what a dApp needs to sign under the stream's
// challenge: the session topics of the kinds its identity may publish.
func newBPSChallengeMsg(req bpsRequest, challenge []byte, isOwner bool) (bpsChallengeMsg, error) {
	msg := bpsChallengeMsg{
		Type:      "challenge",
		Challenge: hex.EncodeToString(challenge),
		Broker:    req.broker.String(),
	}
	type sessionTopic struct {
		kind bps.Kind
		dst  *string
	}
	topics := []sessionTopic{{bps.KindData, &msg.DataTopic}, {bps.KindAuth, &msg.AuthTopic}}
	if isOwner {
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
		Owner common.Address `map:"owner" validate:"required"`
		Topic []byte         `map:"topic" validate:"required,len=32"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return bpsRequest{}, false
	}

	queries := struct {
		Broker       swarm.Address `map:"broker"`
		Identity     []byte        `map:"identity" validate:"omitempty,len=20"`
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
	if len(queries.Identity) == 0 {
		jsonhttp.BadRequest(w, "missing identity")
		return bpsRequest{}, false
	}

	pos, err := bpsPosition(paths.Owner.Bytes(), queries.Cursor, queries.Roster, queries.RosterCursor)
	if err != nil {
		jsonhttp.BadRequest(w, err.Error())
		return bpsRequest{}, false
	}

	return bpsRequest{
		owner:    paths.Owner,
		topic:    paths.Topic,
		broker:   queries.Broker,
		identity: queries.Identity,
		position: pos,
	}, true
}

// bpsPosition builds the place on the channel from the query: the owner's
// cursor, and the roster as address:cursor pairs — the grantees it names, each
// with the lowest index of its updates to deliver — at the roster cursor, the
// lowest roster index to accept. A dApp keeps them from the deliveries it
// received and passes them back when it reconnects.
func bpsPosition(owner []byte, cursor uint64, roster string, rosterCursor uint64) (bps.Position, error) {
	pos := bps.Position{
		Cursors:      map[string]uint64{string(owner): cursor},
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
		if bytes.Equal(a, owner) {
			return pos, fmt.Errorf("%w: the owner must not be on the roster", errBPSInvalidRoster)
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

// bpsCloseCode maps the reason a socket ends to its close code.
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

// bpsClose sends a close frame with the code and the text of err. It may run
// while the inbox writes. Errors are ignored since the connection is torn down
// right after.
func bpsClose(conn *websocket.Conn, err error) {
	reason := err.Error()
	if len(reason) > bpsMaxCloseReason {
		reason = strings.ToValidUTF8(reason[:bpsMaxCloseReason], "")
	}
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(bpsCloseCode(err), reason), time.Now().Add(writeDeadline))
}

// bpsParseFrame splits a frame the dApp sent into kind | index | soc. Whether
// the stream may send it, and whether the chunk is valid, is the session's to say.
func bpsParseFrame(typ int, data []byte) (bps.Kind, uint64, []byte, error) {
	if typ != websocket.BinaryMessage {
		return 0, 0, nil, fmt.Errorf("%w: expected a binary frame", errBPSInvalidMessage)
	}
	if len(data) < bpsFrameHeader {
		return 0, 0, nil, fmt.Errorf("%w: short frame", errBPSInvalidMessage)
	}
	return bps.Kind(data[0]), binary.BigEndian.Uint64(data[1:bpsFrameHeader]), data[bpsFrameHeader:], nil
}

// bpsRun bridges a dApp's socket and its stream to the broker. The identity
// settles what the stream is, as it does on the wire: the owner's publishes once
// it has sent its AUTH, which is due within bpsAuthTimeout; any other listens,
// for as long as it likes, and may send its AUTH once told it is granted. The
// outbox and the inbox run until either ends, or the node shuts down; the first
// reason to stop decides how the socket closes.
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

	isOwner := bytes.Equal(req.identity, req.owner.Bytes())
	authTimer := time.NewTimer(bpsAuthTimeout)
	defer authTimer.Stop()
	if !isOwner {
		authTimer.Stop()
	}

	sess, err := s.bps.Join(ctx, bps.JoinRequest{
		Broker:   req.broker,
		Spec:     bps.CohortSpec{Topic: req.topic, Principal: req.owner.Bytes()},
		Identity: req.identity,
		Position: req.position,
	})
	if err != nil {
		logger.Debug("join failed", "broker", req.broker, "error", err)
		stop(fmt.Errorf("%w: %w", errBPSBrokerGone, err))
		bpsClose(conn, context.Cause(ctx))
		return
	}
	defer sess.Close()

	msg, err := newBPSChallengeMsg(req, sess.Challenge(), isOwner)
	if err != nil {
		logger.Debug("challenge message failed", "error", err)
		bpsClose(conn, fmt.Errorf("%w: %w", errBPSBrokerGone, err))
		return
	}
	if err := s.bpsWriteJSON(conn, msg); err != nil {
		logger.Debug("ws write failed", "error", err)
		return
	}

	var wg sync.WaitGroup
	wg.Go(func() { stop(bpsHandleOutbox(ctx, conn, sess, authTimer)) })
	wg.Go(func() { stop(s.bpsHandleInbox(ctx, conn, sess, isOwner, authTimer.C, logger)) })

	<-ctx.Done()
	if err := context.Cause(ctx); !errors.Is(err, errBPSClientGone) {
		bpsClose(conn, err)
	}
	// ends the outbox's read
	_ = conn.Close()
	wg.Wait()
}

// bpsHandleOutbox takes what the dApp sends out through the session, until the
// socket fails or the session refuses a frame, and returns why it stopped. It
// runs even while the stream may send nothing: only reading sees a client
// that has gone, and a frame sent too early is refused by the session.
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

// bpsHandleInbox brings what the stream receives in to the dApp, until the
// stream or the socket ends or the owner's AUTH is overdue, and returns why it
// stopped. It is the only writer of frames to the socket but the closing one.
func (s *Service) bpsHandleInbox(ctx context.Context, conn *websocket.Conn, sess bps.Session, isOwner bool, authC <-chan time.Time, logger log.Logger) error {
	ticker := time.NewTicker(s.WsPingPeriod)
	defer ticker.Stop()

	// the owner needs no cue; any other stream is told of its
	var granted <-chan struct{}
	if !isOwner {
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
