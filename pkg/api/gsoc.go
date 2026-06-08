// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// SOC field identifiers that can be requested through the SwarmSocFieldsHeader
// to be serialized and channeled on every incoming GSOC chunk.
const (
	socFieldAddress         = "address"
	socFieldRecoveredPubKey = "recoveredpubkey"
	socFieldIdentifier      = "identifier"
	socFieldSignature       = "signature"
	socFieldWrappedAddress  = "wrappedaddress"
	socFieldSpan            = "span"
	socFieldPayload         = "payload"
)

var validSocFields = []string{
	socFieldAddress,
	socFieldRecoveredPubKey,
	socFieldIdentifier,
	socFieldSignature,
	socFieldWrappedAddress,
	socFieldSpan,
	socFieldPayload,
}

// maxSocFieldsSize is the maximum size of a serialized SOC fields message when
// every field is requested: the whole single owner chunk (identifier +
// signature + span + payload, i.e. SocMaxChunkSize) plus the derived metadata
// fields that are not part of the chunk on the wire (soc address, recovered
// public key and wrapped chunk address).
const maxSocFieldsSize = swarm.SocMaxChunkSize +
	swarm.HashSize + // soc address
	soc.OwnerPubKeySize + // recovered public key
	swarm.HashSize // wrapped chunk address

// parseSocFields parses the SwarmSocFieldsHeader value into a list of SOC field
// identifiers. When the header is empty it defaults to the payload field only,
// which preserves backward compatibility.
func parseSocFields(header string) ([]string, error) {
	if strings.TrimSpace(header) == "" {
		return []string{socFieldPayload}, nil
	}

	parts := strings.Split(header, ",")
	fields := make([]string, 0, len(parts))
	for _, p := range parts {
		f := strings.ToLower(strings.TrimSpace(p))
		if f == "" {
			continue
		}
		if !slices.Contains(validSocFields, f) {
			return nil, fmt.Errorf("unknown soc field: %q", p)
		}
		fields = append(fields, f)
	}
	if len(fields) == 0 {
		return []string{socFieldPayload}, nil
	}
	return fields, nil
}

// socFieldsBytes serializes the requested SOC fields in the same order as they
// were provided in the header.
func socFieldsBytes(c *soc.SOC, fields []string) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	for _, f := range fields {
		switch f {
		case socFieldAddress:
			addr, err := c.Address()
			if err != nil {
				return nil, fmt.Errorf("soc address: %w", err)
			}
			buf.Write(addr.Bytes())
		case socFieldRecoveredPubKey:
			buf.Write(c.OwnerPubKey())
		case socFieldIdentifier:
			buf.Write(c.ID())
		case socFieldSignature:
			buf.Write(c.Signature())
		case socFieldWrappedAddress:
			buf.Write(c.WrappedChunk().Address().Bytes())
		case socFieldSpan:
			buf.Write(c.WrappedChunk().Data()[:swarm.SpanSize])
		case socFieldPayload:
			buf.Write(c.WrappedChunk().Data()[swarm.SpanSize:])
		}
	}
	return buf.Bytes(), nil
}

func (s *Service) gsocWsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("gsoc_subscribe").Build()

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}

	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		SocFields         string `map:"Swarm-Soc-Fields"`
		CacheWrappedChunk bool   `map:"Swarm-Cache-Wrapped-Chunk"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	fields, err := parseSocFields(headers.SocFields)
	if err != nil {
		logger.Debug("invalid soc fields header", "error", err)
		logger.Error(nil, "invalid soc fields header")
		jsonhttp.BadRequest(w, "invalid soc fields header")
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize: swarm.SocMaxChunkSize,
		// WriteBufferSize is only an I/O buffer hint; it does not cap the
		// message size. The serialized output can be the whole single owner
		// chunk plus the derived metadata fields (soc address, recovered public
		// key, wrapped chunk address), so size it to that maximum to avoid split
		// writes.
		WriteBufferSize: maxSocFieldsSize,
		CheckOrigin:     s.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("upgrade failed", "error", err)
		logger.Error(nil, "upgrade failed")
		jsonhttp.InternalServerError(w, "upgrade failed")
		return
	}

	s.wsWg.Add(1)
	go s.gsocListeningWs(conn, paths.Address, fields, headers.CacheWrappedChunk)
}

func (s *Service) gsocListeningWs(conn *websocket.Conn, socAddress swarm.Address, fields []string, cacheWrappedChunk bool) {
	defer s.wsWg.Done()

	var (
		dataC       = make(chan []byte)
		gone        = make(chan struct{})
		ticker      = time.NewTicker(s.WsPingPeriod)
		ctx, cancel = context.WithCancel(context.Background()) // for storing cached chunks
		err         error
	)
	defer func() {
		cancel()
		ticker.Stop()
		_ = conn.Close()
	}()
	cleanup := s.gsoc.Subscribe(socAddress, func(c *soc.SOC) {
		if cacheWrappedChunk {
			if err := s.storer.Cache().Put(ctx, c.WrappedChunk()); err != nil {
				s.logger.Debug("gsoc ws: cache wrapped chunk failed", "error", err)
			}
		}

		b, err := socFieldsBytes(c, fields)
		if err != nil {
			s.logger.Debug("gsoc ws: serialize soc fields failed", "error", err)
			return
		}

		select {
		case dataC <- b:
		case <-gone:
			return
		case <-s.quit:
			return
		}
	})

	defer cleanup()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debug("gsoc ws: client gone", "code", code, "message", text)
		close(gone)
		return nil
	})

	for {
		select {
		case b := <-dataC:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("gsoc ws: set write deadline failed", "error", err)
				return
			}

			err = conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				s.logger.Debug("gsoc ws: write message failed", "error", err)
				return
			}

		case <-s.quit:
			// shutdown
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("gsoc ws: set write deadline failed", "error", err)
				return
			}
			err = conn.WriteMessage(websocket.CloseMessage, []byte{})
			if err != nil {
				s.logger.Debug("gsoc ws: write close message failed", "error", err)
			}
			return
		case <-gone:
			// client gone
			return
		case <-ticker.C:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debug("gsoc ws: set write deadline failed", "error", err)
				return
			}
			if err = conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// error encountered while pinging client. client probably gone
				return
			}
		}
	}
}
