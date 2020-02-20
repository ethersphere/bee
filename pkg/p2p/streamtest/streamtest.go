// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package streamtest

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrRecordsNotFound        = errors.New("records not found")
	ErrStreamNotSupported     = errors.New("stream not supported")
	ErrStreamFullcloseTimeout = errors.New("fullclose timeout")
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
	for _, o := range opts {
		o.apply(r)
	}
	return r
}

func (r *Recorder) NewStream(_ context.Context, addr swarm.Address, protocolName, protocolVersion, streamName string) (p2p.Stream, error) {
	recordIn := newRecord()
	recordOut := newRecord()
	cin := make(chan struct{}, 1)
	cout := make(chan struct{}, 1)
	streamOut := newStream(recordIn, recordOut, cin, cout)
	streamIn := newStream(recordOut, recordIn, cout, cin)

	var handler p2p.HandlerFunc
	for _, p := range r.protocols {
		if p.Name == protocolName && p.Version == protocolVersion {
			for _, s := range p.StreamSpecs {
				if s.Name == streamName {
					handler = s.Handler
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
	record := &Record{in: recordIn, out: recordOut}
	go func() {
		err := handler(p2p.Peer{Address: addr}, streamIn)
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
	in     io.WriteCloser
	out    io.ReadCloser
	cin    chan struct{}
	cout   chan struct{}
	closed bool
	mtx    sync.Mutex // guards close
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

func (s *stream) Close() error {
	// lock + closed is used to avoid multiple closing or sending on a closed channel
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.closed {
		return nil
	}

	if err := s.in.Close(); err != nil {
		return err
	}
	if err := s.out.Close(); err != nil {
		return err
	}

	select {
	case s.cin <- struct{}{}:
		close(s.cin)
		s.closed = true
	default:
	}

	return nil
}

func (s *stream) FullClose() error {
	if err := s.Close(); err != nil {
		return err
	}

	select {
	case <-s.cout:
	case <-time.After(1 * time.Second):
		return ErrStreamFullcloseTimeout
	}

	return nil
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
