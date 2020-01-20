package mock

import (
	"context"
	"io"
	"sync"

	"github.com/janos/bee/pkg/p2p"
	ma "github.com/multiformats/go-multiaddr"
)

type Streamer struct {
	In, Out *Buffer
	handler func(p2p.Peer)
}

func NewStreamer(handler func(p2p.Peer)) *Streamer {
	return &Streamer{
		In:      NewBuffer(),
		Out:     NewBuffer(),
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

type Buffer struct {
	b      []byte
	c      int
	closed bool
	cond   *sync.Cond
}

func NewBuffer() *Buffer {
	return &Buffer{
		cond: sync.NewCond(new(sync.Mutex)),
	}
}

func (b *Buffer) Read(p []byte) (n int, err error) {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	for b.c == len(b.b) || b.closed {
		b.cond.Wait()
	}
	end := b.c + len(p)
	if end > len(b.b) {
		end = len(b.b)
	}
	n = copy(p, b.b[b.c:end])
	b.c += n
	if b.closed {
		err = io.EOF
	}
	return n, err
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	defer b.cond.Signal()

	b.b = append(b.b, p...)
	return len(p), nil
}

func (b *Buffer) Close() error {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	defer b.cond.Broadcast()

	b.closed = true
	return nil
}

func (b *Buffer) Bytes() []byte {
	return b.b
}
