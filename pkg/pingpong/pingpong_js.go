//go:build js
// +build js

package pingpong

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pingpong/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/tracing"
)

type Service struct {
	streamer p2p.Streamer
	logger   log.Logger
	tracer   *tracing.Tracer
}

func New(streamer p2p.Streamer, logger log.Logger, tracer *tracing.Tracer) *Service {
	return &Service{
		streamer: streamer,
		logger:   logger.WithName(loggerName).Register(),
		tracer:   tracer,
	}
}

func (s *Service) Ping(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error) {
	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "pingpong-p2p-ping", s.logger)
	defer span.Finish()

	start := time.Now()
	stream, err := s.streamer.NewStream(ctx, address, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return 0, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		go stream.FullClose()
	}()

	w, r := protobuf.NewWriterAndReader(stream)

	var pong pb.Pong
	for _, msg := range msgs {
		if err := w.WriteMsgWithContext(ctx, &pb.Ping{
			Greeting: msg,
		}); err != nil {
			return 0, fmt.Errorf("write message: %w", err)
		}

		if err := r.ReadMsgWithContext(ctx, &pong); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, fmt.Errorf("read message: %w", err)
		}

	}
	return time.Since(start), nil
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.FullClose()

	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "pingpong-p2p-handler", s.logger)
	defer span.Finish()

	var ping pb.Ping
	for {
		if err := r.ReadMsgWithContext(ctx, &ping); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("read message: %w", err)
		}

		if err := w.WriteMsgWithContext(ctx, &pb.Pong{
			Response: "{" + ping.Greeting + "}",
		}); err != nil {
			return fmt.Errorf("write message: %w", err)
		}
	}
	return nil
}
