package mock

import (
	"context"
	"io"
	"sync"

	"github.com/janos/bee/pkg/p2p"
	ma "github.com/multiformats/go-multiaddr"
)

type Recorder struct {
	in, out *record
	handler func(p2p.Peer)
}

func NewRecorder(handler func(p2p.Peer)) *Recorder {
	return &Recorder{
		in:      newRecord(),
		out:     newRecord(),
		handler: handler,
	}
}

func (r *Recorder) NewStream(ctx context.Context, peerID, protocolName, streamName, version string) (p2p.Stream, error) {
	out := newStream(r.in, r.out)
	in := newStream(r.out, r.in)
	go r.handler(p2p.Peer{
		Addr:   ma.StringCast(peerID),
		Stream: in,
	})
	return out, nil
}

func (r *Recorder) In() []byte {
	return r.in.bytes()
}

func (r *Recorder) Out() []byte {
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
	return r.b
}
