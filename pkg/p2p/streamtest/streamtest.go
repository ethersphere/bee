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

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	ErrRecordsNotFound    = errors.New("records not found")
	ErrStreamNotSupported = errors.New("stream not supported")
	ErrStreamClosed       = errors.New("stream closed")

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
	pingErr            func(ma.Multiaddr) (time.Duration, error)
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

func WithPingErr(pingErr func(ma.Multiaddr) (time.Duration, error)) Option {
	return optionFunc(func(r *Recorder) {
		r.pingErr = pingErr
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

func (r *Recorder) Reset() {
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()

	r.records = make(map[string][]*Record)
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
		if err != nil && !errors.Is(err, io.EOF) {
			record.setErr(err)
		}
	}()

	id := addr.String() + p2p.NewSwarmStreamName(protocolName, protocolVersion, streamName)

	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()

	r.records[id] = append(r.records[id], record)
	return streamOut, nil
}

func (r *Recorder) Ping(ctx context.Context, addr ma.Multiaddr) (rtt time.Duration, err error) {
	if r.pingErr != nil {
		return r.pingErr(addr)
	}
	return rtt, err
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

	var recs []*Record
	err := spinlock.Wait(time.Second*time.Duration(timeoutSec), func() bool {
		recs, _ = r.Records(addr, proto, version, stream)
		if l := len(recs); l > msgs {
			t.Fatalf("too many records. want %d got %d", msgs, l)
		} else if msgs > 0 && l == msgs {
			return true
		}
		return false
		// we can be here if msgs == 0 && l == 0
		// or msgs = x && l < x, both cases are fine
		// and we should continue waiting
	})
	if err != nil && msgs > 0 {
		t.Fatal("timed out while waiting for records")
	}

	return recs
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
	closed          bool
	lock            sync.Mutex
}

func newStream(in, out *record) *stream {
	return &stream{in: in, out: out}
}

func (s *stream) Read(p []byte) (int, error) {
	if s.Closed() {
		return 0, ErrStreamClosed
	}

	return s.out.Read(p)
}

func (s *stream) Write(p []byte) (int, error) {
	if s.Closed() {
		return 0, ErrStreamClosed
	}

	return s.in.Write(p)
}

func (s *stream) Headers() p2p.Headers {
	return s.headers
}

func (s *stream) ResponseHeaders() p2p.Headers {
	return s.responseHeaders
}

func (s *stream) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return ErrStreamClosed
	}

	s.closed = true
	s.in.close()

	return nil
}

func (s *stream) Closed() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.closed
}

func (s *stream) FullClose() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return ErrStreamClosed
	}

	s.closed = true
	s.in.close()
	s.out.close()

	return nil
}

func (s *stream) Reset() (err error) {
	return s.FullClose()
}

type record struct {
	b        []byte
	c        int
	lock     sync.Mutex
	dataSigC chan struct{}
	closed   bool
}

func newRecord() *record {
	return &record{
		dataSigC: make(chan struct{}, 16),
	}
}

func (r *record) Read(p []byte) (n int, err error) {
	for r.c == r.bytesSize() {
		_, ok := <-r.dataSigC
		if !ok {
			return 0, io.EOF
		}
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	end := r.c + len(p)
	if end > len(r.b) {
		end = len(r.b)
	}
	n = copy(p, r.b[r.c:end])
	r.c += n

	return n, nil
}

func (r *record) Write(p []byte) (int, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.closed {
		return 0, ErrStreamClosed
	}

	r.b = append(r.b, p...)
	r.dataSigC <- struct{}{}

	return len(p), nil
}

func (r *record) close() {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.closed {
		return
	}

	r.closed = true
	close(r.dataSigC)
}

func (r *record) bytes() []byte {
	return r.b
}

func (r *record) bytesSize() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return len(r.b)
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

func (r *RecorderDisconnecter) Disconnect(overlay swarm.Address, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.disconnected[overlay.String()] = struct{}{}
	return nil
}

func (r *RecorderDisconnecter) Blocklist(overlay swarm.Address, d time.Duration, _ string) error {
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

// NetworkStatus implements p2p.NetworkStatuser interface.
// It always returns p2p.NetworkStatusAvailable.
func (r *RecorderDisconnecter) NetworkStatus() p2p.NetworkStatus {
	return p2p.NetworkStatusAvailable
}
