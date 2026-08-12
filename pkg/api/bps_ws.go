// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// defaultBpsKeepAlive is the ping period of a pubsub websocket session when
// the client does not name one with Swarm-Keep-Alive.
const defaultBpsKeepAlive = 60 * time.Second

// bpsSession is everything the pump and reader goroutines of one pubsub
// websocket session need, assembled by the handler before the upgrade.
type bpsSession struct {
	att          bps.Attachment
	topic        swarm.Address
	owner        []byte
	binding      pb.TopicBinding
	fields       socFields
	cacheWrapped bool
	keepAlive    time.Duration
}

// bpsWsHandler upgrades a client onto one topic of the pubsub bridge. Every
// parameter is parsed, and the bridge attach performed, before the upgrade, so
// a bad request is answered as an HTTP error rather than as a websocket that
// closes immediately.
func (s *Service) bpsWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("bps_subscribe").Build()

	if s.bps == nil {
		jsonhttp.NotImplemented(w, "pubsub not enabled")
		return
	}

	topic, err := bpsResolveTopic(mux.Vars(r)["topic"])
	if err != nil {
		logger.Debug("parse topic failed", "error", err)
		jsonhttp.BadRequest(w, "invalid topic")
		return
	}

	q := r.URL.Query()

	peer, err := ma.NewMultiaddr(q.Get("peer"))
	if err != nil {
		logger.Debug("parse peer failed", "error", err)
		jsonhttp.BadRequest(w, "invalid peer multiaddr")
		return
	}

	spec, err := bpsSpecFromQuery(topic, q)
	if err != nil {
		logger.Debug("assemble cohort spec failed", "error", err)
		jsonhttp.BadRequest(w, "invalid cohort parameters")
		return
	}

	var owner []byte
	if v := q.Get("owner"); v != "" {
		owner, err = bpsParseAddress(v)
		if err != nil {
			logger.Debug("parse owner failed", "error", err)
			jsonhttp.BadRequest(w, "invalid owner")
			return
		}
	}

	keepAlive := defaultBpsKeepAlive
	if s.WsPingPeriod > 0 {
		keepAlive = s.WsPingPeriod
	}
	if v := r.Header.Get(SwarmKeepAliveHeader); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			logger.Debug("parse keep alive failed", "value", v, "error", err)
			jsonhttp.BadRequest(w, "invalid "+SwarmKeepAliveHeader)
			return
		}
		keepAlive = time.Duration(secs) * time.Second
	}

	fields, err := parseSocFields(r.Header.Get(SwarmSocFieldsHeader))
	if err != nil {
		logger.Debug("parse soc fields failed", "error", err)
		jsonhttp.BadRequest(w, "invalid "+SwarmSocFieldsHeader)
		return
	}

	var cacheWrapped bool
	if v := r.Header.Get(SwarmCacheWrappedChunkHeader); v != "" {
		cacheWrapped, err = strconv.ParseBool(v)
		if err != nil {
			logger.Debug("parse cache wrapped chunk failed", "error", err)
			jsonhttp.BadRequest(w, "invalid "+SwarmCacheWrappedChunkHeader)
			return
		}
	}

	att, err := s.bps.Attach(r.Context(), bps.AttachOptions{
		Peer:  peer,
		Topic: topic,
		Spec:  spec,
		Owner: owner,
	})
	if err != nil {
		logger.Debug("attach failed", "topic", topic, "error", err)
		s.bpsAttachError(w, err)
		return
	}

	// The binding decides how an inbound publish frame is parsed. The live
	// session's spec wins over the requested one: a subscriber-turned-publisher
	// brings no spec at all, and a session that is already open is authoritative.
	binding := pb.TopicBinding_ANCHOR
	if live := att.Spec(); live != nil {
		binding = live.GetBinding()
	} else if spec != nil {
		binding = spec.GetBinding()
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkSize,
		WriteBufferSize: swarm.ChunkSize,
		CheckOrigin:     s.checkOrigin,
	}

	// Counted before the upgrade, not after: the client's dial returns as soon
	// as the handshake response is written, so a shutdown that starts right
	// then would race Add against wsWg.Wait and could miss this session.
	s.wsWg.Add(1)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.wsWg.Done()
		logger.Debug("upgrade failed", "error", err)
		logger.Error(nil, "upgrade failed")
		_ = att.Close()
		jsonhttp.InternalServerError(w, "upgrade failed")
		return
	}

	go s.bpsPumpWs(conn, &bpsSession{
		att:          att,
		topic:        topic,
		owner:        owner,
		binding:      binding,
		fields:       fields,
		cacheWrapped: cacheWrapped,
		keepAlive:    keepAlive,
	})
}

// bpsAttachError answers an attach failure. A spec that disagrees with the
// live session is 409 rather than 403: nothing about the client is refused,
// the request conflicts with state the node already holds, and the client can
// retry with the session's own spec.
func (s *Service) bpsAttachError(w http.ResponseWriter, err error) {
	var refusal *bps.RefusalError
	switch {
	case errors.As(err, &refusal):
		switch refusal.Status {
		case pb.Status_FULL:
			jsonhttp.ServiceUnavailable(w, "cohort full")
		case pb.Status_UNKNOWN_TOPIC:
			jsonhttp.NotFound(w, "unknown topic")
		case pb.Status_REJECTED:
			jsonhttp.Forbidden(w, "broker rejected the handshake")
		default:
			jsonhttp.InternalServerError(w, "attach failed")
		}
	case errors.Is(err, bps.ErrSpecMismatch):
		jsonhttp.Conflict(w, "cohort spec mismatch")
	case errors.Is(err, bps.ErrNotPublisher):
		jsonhttp.Forbidden(w, "not a publisher")
	case errors.Is(err, bps.ErrNoPeer),
		errors.Is(err, bps.ErrInvalidSpec),
		errors.Is(err, bps.ErrUnsupportedBinding),
		errors.Is(err, bps.ErrUnsupportedRegime):
		jsonhttp.BadRequest(w, "invalid attach request")
	default:
		jsonhttp.InternalServerError(w, "attach failed")
	}
}

// bpsPumpWs writes the attachment's messages to the client until either side
// goes away. It owns the connection and the attachment from here on.
func (s *Service) bpsPumpWs(conn *websocket.Conn, ss *bpsSession) {
	defer s.wsWg.Done()

	ctx, cancel := context.WithCancel(context.Background())

	var (
		gone   = make(chan struct{})
		once   sync.Once
		ticker = time.NewTicker(ss.keepAlive)
	)
	closeGone := func() { once.Do(func() { close(gone) }) }

	defer func() {
		cancel()
		ticker.Stop()
		_ = conn.Close()
		_ = ss.att.Close()
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debug("bps ws: client gone", "code", code, "message", text)
		closeGone()
		return nil
	})

	// Only a publisher session reads: a subscriber has nothing to send, and a
	// reader would only compete with the library's own control-frame handling.
	if len(ss.owner) > 0 {
		go s.bpsReadWs(ctx, conn, ss, closeGone)
	}

	msgs := ss.att.Messages()

	for {
		select {
		case sc, ok := <-msgs:
			if !ok {
				// the bridge tore the session down
				s.bpsWriteClose(conn)
				return
			}

			if ss.cacheWrapped {
				if err := s.storer.Cache().Put(ctx, sc.WrappedChunk()); err != nil {
					s.logger.Debug("bps ws: cache wrapped chunk failed", "error", err)
				}
			}

			msgType, data, err := serializeSoc(ss.fields, sc)
			if err != nil {
				s.logger.Debug("bps ws: serialize message failed", "error", err)
				continue
			}

			if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
				s.logger.Debug("bps ws: set write deadline failed", "error", err)
				return
			}
			if err := conn.WriteMessage(msgType, data); err != nil {
				s.logger.Debug("bps ws: write message failed", "error", err)
				return
			}

		case <-s.quit:
			s.bpsWriteClose(conn)
			return

		case <-gone:
			return

		case <-ticker.C:
			if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
				s.logger.Debug("bps ws: set write deadline failed", "error", err)
				return
			}
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// client probably gone
				return
			}
		}
	}
}

// bpsWriteClose sends a close frame, best effort.
func (s *Service) bpsWriteClose(conn *websocket.Conn) {
	if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		s.logger.Debug("bps ws: set write deadline failed", "error", err)
		return
	}
	if err := conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
		s.logger.Debug("bps ws: write close message failed", "error", err)
	}
}

// bpsReadWs turns inbound binary frames into publishes. A malformed or refused
// frame is logged and skipped: the websocket carries no per-frame reply, so
// the alternative would be tearing the whole session down over one bad frame.
// It returns — and takes the session down with it — only on a read error,
// which is also how it exits once the pump closes the connection.
func (s *Service) bpsReadWs(ctx context.Context, conn *websocket.Conn, ss *bpsSession, closeGone func()) {
	defer closeGone()

	// Cap the frame gorilla/websocket buffers before parsePublishFrame's own
	// payload-size check ever runs, so an oversized client frame is rejected
	// (ErrReadLimit, session torn down) rather than fully read into memory.
	conn.SetReadLimit(feedIndexSize + swarm.SocSignatureSize + swarm.SpanSize + swarm.ChunkSize)

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			s.logger.Debug("bps ws: read message failed", "error", err)
			return
		}
		if msgType != websocket.BinaryMessage {
			s.logger.Debug("bps ws: ignoring non-binary frame", "type", msgType)
			continue
		}

		sc, err := parsePublishFrame(ss.binding, ss.topic, ss.owner, data)
		if err != nil {
			s.logger.Debug("bps ws: parse publish frame failed", "error", err)
			continue
		}
		if err := ss.att.Publish(ctx, sc); err != nil {
			s.logger.Debug("bps ws: publish failed", "error", err)
			continue
		}
	}
}

// bpsCohortResponse is the cohort spec of one topic in the listing.
type bpsCohortResponse struct {
	Binding       string   `json:"binding"`
	Publishers    string   `json:"publishers"`
	Admin         string   `json:"admin"`
	PublisherList []string `json:"publisherList"`
	Closed        bool     `json:"closed"`
	History       bool     `json:"history"`
}

// bpsTopicResponse is one entry of the GET /pubsub listing.
type bpsTopicResponse struct {
	Topic  string             `json:"topic"`
	Role   string             `json:"role"`
	Peers  int                `json:"peers"`
	Cohort *bpsCohortResponse `json:"cohort,omitempty"`
}

// bpsTopicsHandler lists every topic this node participates in.
func (s *Service) bpsTopicsHandler(w http.ResponseWriter, r *http.Request) {
	if s.bps == nil {
		jsonhttp.NotImplemented(w, "pubsub not enabled")
		return
	}

	status := s.bps.Status()
	out := make([]bpsTopicResponse, 0, len(status))
	for _, t := range status {
		e := bpsTopicResponse{
			Topic: t.Topic.String(),
			Role:  "client",
			Peers: t.Peers,
		}
		if t.Broker {
			e.Role = "broker"
		}
		if t.Spec != nil {
			list := make([]string, 0, len(t.Spec.GetPublisherList()))
			for _, p := range t.Spec.GetPublisherList() {
				list = append(list, hex.EncodeToString(p))
			}
			e.Cohort = &bpsCohortResponse{
				Binding:       bpsBindingName(t.Spec.GetBinding()),
				Publishers:    bpsRegimeName(t.Spec.GetPublishers()),
				Admin:         hex.EncodeToString(t.Spec.GetAdmin()),
				PublisherList: list,
				Closed:        t.Spec.GetClosed(),
				History:       t.Spec.GetHistory(),
			}
		}
		out = append(out, e)
	}

	jsonhttp.OK(w, out)
}

// bpsResolveTopic reads the {topic} path segment: 64 hex characters name the
// topic directly, anything else is a mnemonic hashed into one.
func bpsResolveTopic(raw string) (swarm.Address, error) {
	if raw == "" {
		return swarm.ZeroAddress, fmt.Errorf("bps: empty topic")
	}
	if len(raw) == swarm.HashSize*2 {
		if b, err := hex.DecodeString(raw); err == nil {
			return swarm.NewAddress(b), nil
		}
	}
	h, err := crypto.LegacyKeccak256([]byte(raw))
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("bps: hash topic mnemonic: %w", err)
	}
	return swarm.NewAddress(h), nil
}

// bpsSpecFromQuery assembles a cohort spec from the query, or returns nil when
// the request names no cohort parameter at all — which is a subscribe.
func bpsSpecFromQuery(topic swarm.Address, q map[string][]string) (*pb.CohortSpec, error) {
	get := func(k string) string {
		if v, ok := q[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}

	var named bool
	for _, k := range []string{"binding", "publishers", "admin", "publisher-list", "closed", "history"} {
		if _, ok := q[k]; ok {
			named = true
			break
		}
	}
	if !named {
		return nil, nil
	}

	spec := &pb.CohortSpec{Topic: topic.Bytes()}

	switch get("binding") {
	case "anchor":
		spec.Binding = pb.TopicBinding_ANCHOR
	case "feed":
		spec.Binding = pb.TopicBinding_FEED_TOPIC
	case "":
	default:
		return nil, fmt.Errorf("bps: unknown binding %q", get("binding"))
	}

	switch get("publishers") {
	case "single":
		spec.Publishers = pb.PublisherRegime_EXPLICIT_SINGLE
	case "list":
		spec.Publishers = pb.PublisherRegime_EXPLICIT_LIST
	case "":
	default:
		return nil, fmt.Errorf("bps: unknown publisher regime %q", get("publishers"))
	}

	if v := get("admin"); v != "" {
		admin, err := bpsParseAddress(v)
		if err != nil {
			return nil, fmt.Errorf("bps: admin: %w", err)
		}
		spec.Admin = admin
	}

	if v := get("publisher-list"); v != "" {
		for _, p := range strings.Split(v, ",") {
			addr, err := bpsParseAddress(strings.TrimSpace(p))
			if err != nil {
				return nil, fmt.Errorf("bps: publisher list: %w", err)
			}
			spec.PublisherList = append(spec.PublisherList, addr)
		}
	}

	if v := get("closed"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("bps: closed: %w", err)
		}
		spec.Closed = b
	}

	if v := get("history"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("bps: history: %w", err)
		}
		spec.History = b
	}

	if err := bps.ValidateSpec(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// bpsParseAddress decodes a 20-byte hex ethereum address.
func bpsParseAddress(v string) ([]byte, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(v, "0x"))
	if err != nil {
		return nil, fmt.Errorf("bps: decode address: %w", err)
	}
	if len(b) != crypto.AddressSize {
		return nil, fmt.Errorf("bps: address length %d", len(b))
	}
	return b, nil
}

func bpsBindingName(b pb.TopicBinding) string {
	switch b {
	case pb.TopicBinding_ANCHOR:
		return "anchor"
	case pb.TopicBinding_FEED_TOPIC:
		return "feed"
	default:
		return strings.ToLower(b.String())
	}
}

func bpsRegimeName(p pb.PublisherRegime) string {
	switch p {
	case pb.PublisherRegime_EXPLICIT_SINGLE:
		return "single"
	case pb.PublisherRegime_EXPLICIT_LIST:
		return "list"
	default:
		return strings.ToLower(p.String())
	}
}
