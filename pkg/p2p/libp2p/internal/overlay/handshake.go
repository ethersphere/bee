//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. handshake.proto"
package handshake

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/janos/bee/pkg/p2p"
	"github.com/janos/bee/pkg/p2p/protobuf"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	ProtocolName  = "handshake"
	StreamName    = "handshake"
	StreamVersion = "1.0.0"
)

type Service struct {
	streamerService StreamerService
}

type StreamerService interface {
	Overlay() string
	NewStreamForPeerID(ctx context.Context, id peer.ID, protocolName string, streamName string, version string) (p2p.Stream, error)
	InitPeer(overlay string, peerID peer.ID)
}

func New(libp2pService StreamerService) *Service {
	return &Service{streamerService: libp2pService}
}

func (s *Service) Overlay(ctx context.Context, peerID peer.ID) (overlay string, err error) {
	stream, err := s.streamerService.NewStreamForPeerID(ctx, peerID, ProtocolName, StreamName, StreamVersion)
	if err != nil {
		return "", fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)

	var resp ShakeHand
	if err := w.WriteMsg(&ShakeHand{Address: s.streamerService.Overlay()}); err != nil {
		return "", fmt.Errorf("overlay handler: write message: %v\n", err)
	}

	log.Printf("sent overlay req %s\n", s.streamerService.Overlay())

	if err := r.ReadMsg(&resp); err != nil {
		if err == io.EOF {
			return "", nil
		}

		return "", fmt.Errorf("overlay handler: read message: %v\n", err)
	}

	log.Printf("read overlay resp: %s\n", resp.Address)
	return resp.Address, nil
}

func (s *Service) Handler(stream p2p.Stream, peerID peer.ID) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var req ShakeHand
	if err := r.ReadMsg(&req); err != nil {
		if err == io.EOF {
			return
		}
		log.Printf("overlay handler: read message: %v\n", err)
		return
	}

	log.Printf("received overlay req %s\n", req.Address)
	if err := w.WriteMsg(&ShakeHand{
		Address: s.streamerService.Overlay(),
	}); err != nil {
		log.Printf("overlay handler: write message: %v\n", err)
	}

	s.streamerService.InitPeer(req.Address, peerID)
	log.Printf("sent overlay resp: %s\n", s.streamerService.Overlay())
}
