// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package streamtest

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/ethersphere/bee/pkg/p2p"
)

type Recorder struct {
	records     map[string][]Record
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
		records: make(map[string][]Record),
	}
	for _, o := range opts {
		o.apply(r)
	}
	return r
}

func (r *Recorder) NewStream(_ context.Context, overlay, protocolName, streamName, version string) (p2p.Stream, error) {
	recordIn := newRecord()
	recordOut := newRecord()
	streamOut := newStream(recordIn, recordOut)
	streamIn := newStream(recordOut, recordIn)

	var handler p2p.HandlerFunc
	for _, p := range r.protocols {
		if p.Name == protocolName {
			for _, s := range p.StreamSpecs {
				if s.Name == streamName && s.Version == version {
					handler = s.Handler
				}
			}
		}
	}
	if handler == nil {
		return nil, fmt.Errorf("unsupported protocol stream %q %q %q", protocolName, streamName, version)
	}
	for _, m := range r.middlewares {
		handler = m(handler)
	}
	go func() {
		if err := handler(p2p.Peer{Address: overlay}, streamIn); err != nil {
			panic(err) // todo: store error and export error records for inspection
		}
	}()

	id := overlay + p2p.NewSwarmStreamName(protocolName, streamName, version)

	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()

	r.records[id] = append(r.records[id], Record{in: recordIn, out: recordOut})
	return streamOut, nil
}

func (r *Recorder) Records(peerID, protocolName, streamName, version string) ([]Record, error) {
	id := peerID + p2p.NewSwarmStreamName(protocolName, streamName, version)

	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()

	records, ok := r.records[id]
	if !ok {
		return nil, fmt.Errorf("records not found for %q %q %q %q", peerID, protocolName, streamName, version)
	}
	return records, nil
}

type Record struct {
	in  *record
	out *record
}

func (r *Record) In() []byte {
	return r.in.bytes()
}

func (r *Record) Out() []byte {
	return r.out.bytes()
}

type stream struct {
	in  io.WriteCloser
	out io.ReadCloser
}

func newStream(in io.WriteCloser, out io.ReadCloser) *stream {
	return &stream{in: in, out: out}
}

func (s *stream) Read(p []byte) (int, error) {
	return s.out.Read(p)
}

func (s *stream) Write(p []byte) (int, error) {
	return s.in.Write(p)
}

func (s *stream) Close() error {
	if err := s.in.Close(); err != nil {
		return err
	}
	if err := s.out.Close(); err != nil {
		return err
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

	for r.c == len(r.b) || r.closed {
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
