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
	ErrStreamFullcloseTimeout = errors.New("fullclose timeout")
	fullCloseTimeout          = fullCloseTimeoutDefault // timeout of fullclose
	fullCloseTimeoutDefault   = 5 * time.Second         // default timeout used for helper function to reset timeout when changed

	noopMiddleware = func(f p2p.HandlerFunc) p2p.HandlerFunc {
		return f
	}
)

type Recorder struct {
	records     map[string][]*Record
	recordsMu   sync.Mutex
	protocols   []p2p.ProtocolSpec
	middlewares []p2p.HandlerMiddleware
}

func WithProtocols(protocols ...p2p.ProtocolSpec) Option {
	return optionFunc(func(r *Recorder) {
		r.protocols = append(r.protocols, protocols...)
	})
}

func WithMiddlewares(middlewares ...p2p.HandlerMiddleware) Option {
	return optionFunc(func(r *Recorder) {
		r.middlewares = append(r.middlewares, middlewares...)
	})
}

func New(opts ...Option) *Recorder {
	r := &Recorder{
		records: make(map[string][]*Record),
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
	recordIn := newRecord()
	recordOut := newRecord()
	closedIn := make(chan struct{})
	closedOut := make(chan struct{})
	streamOut := newStream(recordIn, recordOut, closedIn, closedOut)
	streamIn := newStream(recordOut, recordIn, closedOut, closedIn)

	var handler p2p.HandlerFunc
	var headler p2p.HeadlerFunc
	for _, p := range r.protocols {
		if p.Name == protocolName && p.Version == protocolVersion {
			for _, s := range p.StreamSpecs {
				if s.Name == streamName {
					handler = s.Handler
					headler = s.Headler
				}
			}
		}
	}
	if handler == nil {
		return nil, ErrStreamNotSupported
	}
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		handler = r.middlewares[i](handler)
	}
	if headler != nil {
		streamOut.headers = headler(h)
	}
	record := &Record{in: recordIn, out: recordOut}
	go func() {
		err := handler(ctx, p2p.Peer{Address: addr}, streamIn)
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
	in        io.WriteCloser
	out       io.ReadCloser
	headers   p2p.Headers
	cin       chan struct{}
	cout      chan struct{}
	closeOnce sync.Once
}

func newStream(in io.WriteCloser, out io.ReadCloser, cin, cout chan struct{}) *stream {
	return &stream{in: in, out: out, cin: cin, cout: cout}
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

func (s *stream) Close() error {
	var e error
	s.closeOnce.Do(func() {
		if err := s.in.Close(); err != nil {
			e = err
			return
		}
		if err := s.out.Close(); err != nil {
			e = err
			return
		}

		close(s.cin)
	})

	return e
}

func (s *stream) FullClose() error {
	if err := s.Close(); err != nil {
		return err
	}

	select {
	case <-s.cout:
	case <-time.After(fullCloseTimeout):
		return ErrStreamFullcloseTimeout
	}

	return nil
}

func (s *stream) Reset() error {
	//todo: :implement appropriately after all protocols are migrated and tested
	return s.Close()
}

type record struct {
	b      []byte
	c      int
	closed bool
	cond   *sync.Cond
}

func newRecord() *record {
	return &record{
		cond: sync.NewCond(new(sync.Mutex)),
	}
}

func (r *record) Read(p []byte) (n int, err error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	for r.c == len(r.b) && !r.closed {
		r.cond.Wait()
	}
	end := r.c + len(p)
	if end > len(r.b) {
		end = len(r.b)
	}
	n = copy(p, r.b[r.c:end])
	r.c += n
	if r.closed {
		err = io.EOF
	}
	return n, err
}

func (r *record) Write(p []byte) (int, error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	defer r.cond.Signal()

	r.b = append(r.b, p...)
	return len(p), nil
}

func (r *record) Close() error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	defer r.cond.Broadcast()

	r.closed = true
	return nil
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
