package mock

import (
	"context"
	"io"
	"sync"

	"github.com/janos/bee/pkg/p2p"
	ma "github.com/multiformats/go-multiaddr"
)

type Streamer struct {
	In, Out *Recorder
	handler func(p2p.Peer)
}

func NewStreamer(handler func(p2p.Peer)) *Streamer {
	return &Streamer{
		In:      NewRecorder(),
		Out:     NewRecorder(),
		handler: handler,
	}
}

func (s *Streamer) NewStream(ctx context.Context, peerID, protocolName, streamName, version string) (p2p.Stream, error) {
	out := NewStream(s.In, s.Out)
	in := NewStream(s.Out, s.In)
	go s.handler(p2p.Peer{
		Addr:   ma.StringCast(peerID),
		Stream: in,
	})
	return out, nil
}

type Stream struct {
	in  io.WriteCloser
	out io.ReadCloser
}

func NewStream(in io.WriteCloser, out io.ReadCloser) *Stream {
	return &Stream{in: in, out: out}
}

func (s *Stream) Read(p []byte) (int, error) {
	return s.out.Read(p)
}

func (s *Stream) Write(p []byte) (int, error) {
	return s.in.Write(p)
}

func (s *Stream) Close() error {
	if err := s.in.Close(); err != nil {
		return err
	}
	if err := s.out.Close(); err != nil {
		return err
	}
	return nil
}

type Recorder struct {
	b      []byte
	c      int
	closed bool
	cond   *sync.Cond
}

func NewRecorder() *Recorder {
	return &Recorder{
		cond: sync.NewCond(new(sync.Mutex)),
	}
}

func (r *Recorder) Read(p []byte) (n int, err error) {
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

func (r *Recorder) Write(p []byte) (int, error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	defer r.cond.Signal()

	r.b = append(r.b, p...)
	return len(p), nil
}

func (r *Recorder) Close() error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	defer r.cond.Broadcast()

	r.closed = true
	return nil
}

func (r *Recorder) Bytes() []byte {
	return r.b
}
