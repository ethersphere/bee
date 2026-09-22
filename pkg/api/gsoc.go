// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
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

// gsocQueueCapacity is the maximum number of pending outgoing messages held
// per GSOC websocket subscription. Since a serialized message is at most
// maxSocFieldsSize bytes, this caps a single subscription's backlog at a bit
// over 1 MB, while still leaving enough room to absorb the bursts a client
// that keeps reading can be expected to work through.
const gsocQueueCapacity = 256

// socFieldSizes is the single source of truth for the valid SOC fields: it maps
// every field identifier to the maximum number of bytes its serialized form
// occupies. A field added here is accepted by parseSocFields and accounted for
// in maxSocFieldsSize without any further change.
var socFieldSizes = map[string]int{
	socFieldAddress:         swarm.HashSize,
	socFieldRecoveredPubKey: soc.OwnerPubKeySize,
	socFieldIdentifier:      swarm.HashSize,
	socFieldSignature:       swarm.SocSignatureSize,
	socFieldWrappedAddress:  swarm.HashSize,
	socFieldSpan:            swarm.SpanSize,
	socFieldPayload:         swarm.ChunkSize,
}

// maxSocFieldsSize is the maximum size of a serialized SOC fields message when
// every field is requested. It is derived from socFieldSizes so that it stays
// correct when fields are added or removed.
var maxSocFieldsSize = func() (size int) {
	for _, s := range socFieldSizes {
		size += s
	}
	return size
}()

// parseSocFields parses the SwarmSocFieldsHeader value into a list of SOC field
// identifiers. When the header is empty it defaults to the payload field only,
// which preserves backward compatibility. Duplicate fields are dropped, keeping
// the first occurrence, so the returned slice never exceeds len(socFieldSizes)
// entries regardless of how many times a field is repeated in the header.
func parseSocFields(header string) ([]string, error) {
	if strings.TrimSpace(header) == "" {
		return []string{socFieldPayload}, nil
	}

	seen := make(map[string]bool, len(socFieldSizes))
	parts := strings.Split(header, ",")
	fields := make([]string, 0, len(socFieldSizes))
	for _, p := range parts {
		f := strings.ToLower(strings.TrimSpace(p))
		if f == "" {
			continue
		}
		if _, ok := socFieldSizes[f]; !ok {
			return nil, fmt.Errorf("unknown soc field: %q", p)
		}
		if seen[f] {
			continue
		}
		seen[f] = true
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

	// Subscribe synchronously, before handing the connection off to its own
	// goroutine: Upgrade already flushed the 101 response, so the client can
	// start sending GSOC-triggering activity immediately. Subscribing here
	// instead of inside the spawned goroutine closes the window in which an
	// update could arrive before the handler is registered and be silently
	// missed.
	queue := newGsocQueue()
	wake := make(chan struct{}, 1)
	cleanup := s.gsoc.Subscribe(paths.Address, func(c *soc.SOC) {
		if headers.CacheWrappedChunk {
			// Caching is a node-local side effect independent of this
			// subscriber's connection, so it must not be aborted just
			// because the websocket closes mid-write.
			if err := s.storer.Cache().Put(context.Background(), c.WrappedChunk()); err != nil {
				s.logger.Debug("gsoc ws: cache wrapped chunk failed", "error", err)
			}
		}

		b, err := socFieldsBytes(c, fields)
		if err != nil {
			s.logger.Warning("gsoc ws: serialize soc fields failed", "error", err)
			return
		}

		queue.push(b)

		// Non-blocking: the writer only needs to know there is something to
		// drain, not one notification per message, so a full wake channel
		// means it is already going to pick this up.
		select {
		case wake <- struct{}{}:
		default:
		}
	})

	s.wsWg.Add(1)
	go s.gsocListeningWs(conn, cleanup, queue, wake)
}

// gsocQueue is a bounded FIFO ring buffer of pending outgoing GSOC messages.
//
// The producer (the GSOC subscription callback) runs on the node's chunk
// handling path, so it must never block on the websocket writer; with an
// unbounded queue that would let anyone spamming a subscribed GSOC address
// grow the backlog without limit and exhaust the node's memory whenever the
// client does not keep up. Once the queue is full the oldest pending message
// is therefore evicted to make room for the newest one: for a real-time
// subscription a fresh update is worth more than a stale one.
type gsocQueue struct {
	mu       sync.Mutex
	items    [][]byte // ring buffer, fixed length gsocQueueCapacity
	head     int      // index of the oldest queued message
	size     int      // number of queued messages
	dropped  uint64   // messages evicted since the last droppedCount call
	released bool     // set once the writer is gone, see release
}

func newGsocQueue() *gsocQueue {
	return &gsocQueue{items: make([][]byte, gsocQueueCapacity)}
}

// push queues a message, evicting the oldest one if the queue is full. It is a
// no-op once the queue has been released.
func (q *gsocQueue) push(b []byte) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.released {
		return
	}
	if q.size == len(q.items) {
		q.items[q.head] = nil
		q.head = (q.head + 1) % len(q.items)
		q.size--
		q.dropped++
	}
	q.items[(q.head+q.size)%len(q.items)] = b
	q.size++
}

// pop returns the oldest queued message, or ok=false if the queue is empty.
func (q *gsocQueue) pop() (b []byte, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size == 0 {
		return nil, false
	}
	b = q.items[q.head]
	q.items[q.head] = nil // drop the reference so the message can be collected
	q.head = (q.head + 1) % len(q.items)
	q.size--
	return b, true
}

// droppedCount returns how many messages were evicted since the previous call
// and resets the counter, so that a slow subscriber is reported once per drain
// instead of once per lost message.
func (q *gsocQueue) droppedCount() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()

	dropped := q.dropped
	q.dropped = 0
	return dropped
}

// release discards the undelivered backlog and stops the queue from accepting
// further messages. Nothing drains the queue once its writer is gone, so the
// pending messages are dead weight from that point on; dropping them here
// frees them right away instead of keeping them alive for as long as a
// producer that is still mid-callback can reach the queue.
func (q *gsocQueue) release() {
	q.mu.Lock()
	defer q.mu.Unlock()

	clear(q.items)
	q.head = 0
	q.size = 0
	q.released = true
}

func (s *Service) gsocListeningWs(conn *websocket.Conn, cleanup func(), queue *gsocQueue, wake chan struct{}) {
	defer s.wsWg.Done()
	// Defers run in reverse order: unsubscribe first, so that no producer can
	// queue anything new, and only then drop whatever backlog this connection
	// never got to write out.
	defer queue.release()
	defer cleanup()

	var (
		gone   = make(chan struct{})
		ticker = time.NewTicker(s.WsPingPeriod)
		err    error
	)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debug("gsoc ws: client gone", "code", code, "message", text)
		close(gone)
		return nil
	})

	for {
		select {
		case <-wake:
			for {
				// Draining a backlog must not outlast the node. A consumer
				// that keeps every write just under the write deadline makes
				// each message cost seconds, so a full queue would otherwise
				// hold this goroutine for minutes: long past the second that
				// Close waits for it, leaving the shutdown to report open
				// websockets and starving the keepalive ping in the meantime.
				// Re-check the exits between messages to bound that to the
				// single write already in flight.
				select {
				case <-s.quit:
					s.gsocWsNotifyClose(conn)
					return
				case <-gone:
					return
				default:
				}

				b, ok := queue.pop()
				if !ok {
					break
				}

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
			}

			if dropped := queue.droppedCount(); dropped > 0 {
				s.logger.Warning("gsoc ws: subscriber too slow, messages dropped", "count", dropped)
			}

		case <-s.quit:
			// shutdown
			s.gsocWsNotifyClose(conn)
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

// gsocWsNotifyClose tells the subscriber that the node is going away. It is
// best effort: the connection is closed either way once the writer returns.
func (s *Service) gsocWsNotifyClose(conn *websocket.Conn) {
	if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		s.logger.Debug("gsoc ws: set write deadline failed", "error", err)
		return
	}
	if err := conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
		s.logger.Debug("gsoc ws: write close message failed", "error", err)
	}
}
