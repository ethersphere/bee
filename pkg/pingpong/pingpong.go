//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. pingpong.proto"

package pingpong

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/janos/bee/pkg/p2p"
	"github.com/janos/bee/pkg/p2p/protobuf"
)

const (
	protocolName  = "pingpong"
	streamName    = "pingpong"
	streamVersion = "1.0.0"
)

type Service struct {
	p2p p2p.Service
}

func New(p2ps p2p.Service) (s *Service, err error) {
	s = &Service{
		p2p: p2ps,
	}

	if err := p2ps.AddProtocol(p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Version: streamVersion,
				Handler: s.Handler,
			},
		},
	}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Handler(p p2p.Peer) {
	w, r := protobuf.NewRW(p.Stream)
	defer p.Stream.Close()

	var ping Ping
	for {
		if err := r.ReadMsg(&ping); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("pingpong handler: read message: %v\n", err)
			return
		}
		log.Printf("got ping: %q\n", ping.Greeting)

		if err := w.WriteMsg(&Pong{
			Response: ping.Greeting,
		}); err != nil {
			log.Printf("pingpong handler: write message: %v\n", err)
			return
		}
	}
}

func (s *Service) Ping(ctx context.Context, peerID string, msgs ...string) (rtt time.Duration, err error) {
	stream, err := s.p2p.NewStream(ctx, peerID, protocolName, streamName, streamVersion)
	if err != nil {
		return 0, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewRW(stream)

	var pong Pong
	start := time.Now()
	for _, msg := range msgs {
		if err := w.WriteMsg(&Ping{
			Greeting: msg,
		}); err != nil {
			return 0, fmt.Errorf("stream write: %w", err)
		}

		if err := r.ReadMsg(&pong); err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		log.Printf("got pong: %q\n", pong.Response)
	}
	return time.Since(start) / time.Duration(len(msgs)), nil
}
