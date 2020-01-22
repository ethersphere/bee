//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. overlay.proto"
package overlay

import (
	"context"
	"fmt"
	"github.com/janos/bee/pkg/p2p"
	"github.com/janos/bee/pkg/p2p/protobuf"
	"io"
	"log"
)

const (
	protocolName  = "overlay"
	streamName    = "overlay"
	streamVersion = "1.0.0"
)

type Service struct {
	streamer p2p.Streamer
}

func New(streamer p2p.Streamer) *Service {
	return &Service{streamer: streamer}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Version: streamVersion,
				Handler: s.Handler,
			},
		},
	}
}

func (s *Service) Handler(p p2p.Peer) {
	w, r := protobuf.NewWriterAndReader(p.Stream)
	defer p.Stream.Close()

	var req OverlayReq
	if err := r.ReadMsg(&req); err != nil {
		if err == io.EOF {
			return
		}
		log.Printf("overlay handler: read message: %v\n", err)
		return
	}

	log.Println("received overlay req")

	if err := w.WriteMsg(&OverlayResp{
		Address: p.OverlayAddr,
	}); err != nil {
		log.Printf("overlay handler: write message: %v\n", err)
	}

	log.Printf("sent overlay resp: %s\n", p.OverlayAddr)

}

func (s *Service) Overlay(ctx context.Context, peerID string) (err error) {
	stream, err := s.streamer.NewStream(ctx, peerID, protocolName, streamName, streamVersion)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)

	var resp OverlayResp
	if err := w.WriteMsg(&OverlayReq{}); err != nil {
		return fmt.Errorf("overlay handler: write message: %v\n", err)
	}

	log.Println("sent overlay req")


	if err := r.ReadMsg(&resp); err != nil {
		if err == io.EOF {
			 return nil
		}

		return fmt.Errorf("overlay handler: read message: %v\n", err)
	}


	log.Printf("sent overlay resp: %s\n", resp.Address)
	return nil
}
