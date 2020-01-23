// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/janos/bee/pkg/p2p"
	ma "github.com/multiformats/go-multiaddr"
)

type Recorder struct {
	records   map[string][]Record
	recordsMu sync.Mutex
	protocols []p2p.ProtocolSpec
}

func NewRecorder(protocols ...p2p.ProtocolSpec) *Recorder {
	return &Recorder{
		records:   make(map[string][]Record),
		protocols: protocols,
	}
}

func (r *Recorder) NewStream(_ context.Context, peerID, protocolName, streamName, version string) (p2p.Stream, error) {
	recordIn := newRecord()
	recordOut := newRecord()
	streamOut := newStream(recordIn, recordOut)
	streamIn := newStream(recordOut, recordIn)

	var handler func(p2p.Peer)
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

	go handler(p2p.Peer{
		Addr:   ma.StringCast(peerID),
		Stream: streamIn,
	})

	id := peerID + p2p.NewSwarmStreamName(protocolName, streamName, version)

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
