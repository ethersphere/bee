// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package streamtest

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrRecordsNotFound        = errors.New("records not found")
	ErrStreamNotSupported     = errors.New("stream not supported")
	ErrStreamClosed           = errors.New("stream closed")
	ErrStreamFullcloseTimeout = errors.New("fullclose timeout")
	fullCloseTimeout          = fullCloseTimeoutDefault // timeout of fullclose
	fullCloseTimeoutDefault   = 5 * time.Second         // default timeout used for helper function to reset timeout when changed

	noopMiddleware = func(f p2p.HandlerFunc) p2p.HandlerFunc {
		return f
	}
)

type Recorder struct {
	base               swarm.Address
	fullNode           bool
	records            map[string][]*Record
	recordsMu          sync.Mutex
	protocols          []p2p.ProtocolSpec
	middlewares        []p2p.HandlerMiddleware
	streamErr          func(swarm.Address, string, string, string) error
	protocolsWithPeers map[string]p2p.ProtocolSpec
}

func WithProtocols(protocols ...p2p.ProtocolSpec) Option {
	return optionFunc(func(r *Recorder) {
		r.protocols = append(r.protocols, protocols...)
	})
}

func WithPeerProtocols(protocolsWithPeers map[string]p2p.ProtocolSpec) Option {
	return optionFunc(func(r *Recorder) {
		r.protocolsWithPeers = protocolsWithPeers
	})
}

func WithMiddlewares(middlewares ...p2p.HandlerMiddleware) Option {
	return optionFunc(func(r *Recorder) {
		r.middlewares = append(r.middlewares, middlewares...)
	})
}

func WithBaseAddr(a swarm.Address) Option {
	return optionFunc(func(r *Recorder) {
		r.base = a
	})
}

func WithLightNode() Option {
	return optionFunc(func(r *Recorder) {
		r.fullNode = false
	})
}

func WithStreamError(streamErr func(swarm.Address, string, string, string) error) Option {
	return optionFunc(func(r *Recorder) {
		r.streamErr = streamErr
	})
}

func New(opts ...Option) *Recorder {
	r := &Recorder{
		records:  make(map[string][]*Record),
		fullNode: true,
	}

	r.middlewares = append(r.middlewares, noopMiddleware)

	for _, o := range opts {
		o.apply(r)
	}
	return r
}

func (r *Recorder) SetProtocols(protocols ...p2p.ProtocolSpec) {
	r.protocols = append(r.protocols, protocols...)
}

func (r *Recorder) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocolName, protocolVersion, streamName string) (p2p.Stream, error) {
	if r.streamErr != nil {
		err := r.streamErr(addr, protocolName, protocolVersion, streamName)
		if err != nil {
			return nil, err
		}
	}
	recordIn := newRecord()
	recordOut := newRecord()
	streamOut := newStream(recordIn, recordOut)
	streamIn := newStream(recordOut, recordIn)

	var handler p2p.HandlerFunc
	var headler p2p.HeadlerFunc
	peerHandlers, ok := r.protocolsWithPeers[addr.String()]
	if !ok {
		for _, p := range r.protocols {
			if p.Name == protocolName && p.Version == protocolVersion {
				peerHandlers = p
			}
		}
	}
	for _, s := range peerHandlers.StreamSpecs {
		if s.Name == streamName {
			handler = s.Handler
			headler = s.Headler
		}
	}
	if handler == nil {
		return nil, ErrStreamNotSupported
	}
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		handler = r.middlewares[i](handler)
	}
	if headler != nil {
		streamOut.headers = headler(h, addr)
	}
	record := &Record{in: recordIn, out: recordOut, done: make(chan struct{})}
	go func() {
		defer close(record.done)

		// pass a new context to handler,
		streamIn.responseHeaders = streamOut.headers
		// do not cancel it with the client stream context
		err := handler(context.Background(), p2p.Peer{Address: r.base, FullNode: r.fullNode}, streamIn)
		if err != nil && err != io.EOF {
			record.setErr(err)
		}
	}()

	id := addr.String() + p2p.NewSwarmStreamName(protocolName, protocolVersion, streamName)

	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()

	r.records[id] = append(r.records[id], record)
	return streamOut, nil
}

func (r *Recorder) Records(addr swarm.Address, protocolName, protocolVersio, streamName string) ([]*Record, error) {
	id := addr.String() + p2p.NewSwarmStreamName(protocolName, protocolVersio, streamName)

	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()

	records, ok := r.records[id]
	if !ok {
		return nil, ErrRecordsNotFound
	}
	// wait for all records goroutines to terminate
	for _, r := range records {
		<-r.done
	}
	return records, nil
}

// WaitRecords waits for some time for records to come into the recorder. If msgs is 0, the timeoutSec period is waited to verify
// that _no_ messages arrive during this time period.
func (r *Recorder) WaitRecords(t *testing.T, addr swarm.Address, proto, version, stream string, msgs, timeoutSec int) []*Record {
	t.Helper()
	wait := 10 * time.Millisecond
	iters := int((time.Duration(timeoutSec) * time.Second) / wait)

	for i := 0; i < iters; i++ {
		recs, _ := r.Records(addr, proto, version, stream)
		if l := len(recs); l > msgs {
			t.Fatalf("too many records. want %d got %d", msgs, l)
		} else if msgs > 0 && l == msgs {
			return recs
		}
		// we can be here if msgs == 0 && l == 0
		// or msgs = x && l < x, both cases are fine
		// and we should continue waiting

		time.Sleep(wait)
	}
	if msgs > 0 {
		t.Fatal("timed out while waiting for records")
	}
	return nil
}

type Record struct {
	in    *record
	out   *record
	err   error
	errMu sync.Mutex
	done  chan struct{}
}

func (r *Record) In() []byte {
	return r.in.bytes()
}

func (r *Record) Out() []byte {
	return r.out.bytes()
}

func (r *Record) Err() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()

	return r.err
}

func (r *Record) setErr(err error) {
	r.errMu.Lock()
	defer r.errMu.Unlock()

	r.err = err
}

type stream struct {
	in              *record
	out             *record
	headers         p2p.Headers
	responseHeaders p2p.Headers
}

func newStream(in, out *record) *stream {
	return &stream{in: in, out: out}
}

func (s *stream) Read(p []byte) (int, error) {
	return s.out.Read(p)
}

func (s *stream) Write(p []byte) (int, error) {
	return s.in.Write(p)
}

func (s *stream) Headers() p2p.Headers {
	return s.headers
}

func (s *stream) ResponseHeaders() p2p.Headers {
	return s.responseHeaders
}

func (s *stream) Close() error {
	return s.in.Close()
}

func (s *stream) FullClose() error {
	if err := s.Close(); err != nil {
		_ = s.Reset()
		return err
	}

	waitStart := time.Now()

	for {
		if s.out.Closed() {
			return nil
		}

		if time.Since(waitStart) >= fullCloseTimeout {
			return ErrStreamFullcloseTimeout
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func (s *stream) Reset() (err error) {
	if err := s.in.Close(); err != nil {
		_ = s.out.Close()
		return err
	}

	return s.out.Close()
}

type record struct {
	b       []byte
	c       int
	closed  bool
	closeMu sync.RWMutex
	cond    *sync.Cond
}

func newRecord() *record {
	return &record{
		cond: sync.NewCond(new(sync.Mutex)),
	}
}

func (r *record) Read(p []byte) (n int, err error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	for r.c == len(r.b) && !r.Closed() {
		r.cond.Wait()
	}
	end := r.c + len(p)
	if end > len(r.b) {
		end = len(r.b)
	}
	n = copy(p, r.b[r.c:end])
	r.c += n
	if r.Closed() {
		err = io.EOF
	}

	return n, err
}

func (r *record) Write(p []byte) (int, error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	if r.Closed() {
		return 0, ErrStreamClosed
	}

	defer r.cond.Signal()

	r.b = append(r.b, p...)
	return len(p), nil
}

func (r *record) Close() error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	defer r.cond.Broadcast()

	r.closeMu.Lock()
	r.closed = true
	r.closeMu.Unlock()

	return nil
}

func (r *record) Closed() bool {
	r.closeMu.RLock()
	defer r.closeMu.RUnlock()
	return r.closed
}

func (r *record) bytes() []byte {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	return r.b
}

type Option interface {
	apply(*Recorder)
}
type optionFunc func(*Recorder)

func (f optionFunc) apply(r *Recorder) { f(r) }

var _ p2p.StreamerDisconnecter = (*RecorderDisconnecter)(nil)

type RecorderDisconnecter struct {
	*Recorder
	disconnected map[string]struct{}
	blocklisted  map[string]time.Duration
	mu           sync.RWMutex
}

func NewRecorderDisconnecter(r *Recorder) *RecorderDisconnecter {
	return &RecorderDisconnecter{
		Recorder:     r,
		disconnected: make(map[string]struct{}),
		blocklisted:  make(map[string]time.Duration),
	}
}

func (r *RecorderDisconnecter) Disconnect(overlay swarm.Address) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.disconnected[overlay.String()] = struct{}{}
	return nil
}

func (r *RecorderDisconnecter) Blocklist(overlay swarm.Address, d time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.blocklisted[overlay.String()] = d
	return nil
}

func (r *RecorderDisconnecter) IsDisconnected(overlay swarm.Address) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, yes := r.disconnected[overlay.String()]
	return yes
}

func (r *RecorderDisconnecter) IsBlocklisted(overlay swarm.Address) (bool, time.Duration) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, yes := r.blocklisted[overlay.String()]
	return yes, d
}
