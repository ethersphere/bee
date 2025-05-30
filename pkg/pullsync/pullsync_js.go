//go:build js
// +build js

package pullsync

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/ratelimit"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"resenje.org/singleflight"
)

type Syncer struct {
	streamer       p2p.Streamer
	logger         log.Logger
	store          storer.Reserve
	quit           chan struct{}
	unwrap         func(swarm.Chunk)
	gsocHandler    func(*soc.SOC)
	validStamp     postage.ValidStampFn
	intervalsSF    singleflight.Group[string, *collectAddrsResult]
	syncInProgress atomic.Int32

	maxPage uint64

	limiter *ratelimit.Limiter

	Interface
	io.Closer
}

func New(
	streamer p2p.Streamer,
	store storer.Reserve,
	unwrap func(swarm.Chunk),
	gsocHandler func(*soc.SOC),
	validStamp postage.ValidStampFn,
	logger log.Logger,
	maxPage uint64,
) *Syncer {

	return &Syncer{
		streamer:    streamer,
		store:       store,
		unwrap:      unwrap,
		gsocHandler: gsocHandler,
		validStamp:  validStamp,
		logger:      logger.WithName(loggerName).Register(),
		quit:        make(chan struct{}),
		maxPage:     maxPage,
		limiter:     ratelimit.New(handleRequestsLimitRate, int(maxPage)),
	}
}

func (s *Syncer) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Handler: s.handler,
			},
			{
				Name:    cursorStreamName,
				Handler: s.cursorHandler,
			},
		},
		DisconnectIn:  s.disconnect,
		DisconnectOut: s.disconnect,
	}
}

// handler handles an incoming request to sync an interval
func (s *Syncer) handler(streamCtx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {

	select {
	case <-s.quit:
		return nil
	default:
		s.syncInProgress.Add(1)
		defer s.syncInProgress.Add(-1)
	}

	r := protobuf.NewReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	ctx, cancel := context.WithCancel(streamCtx)
	defer cancel()

	go func() {
		select {
		case <-s.quit:
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	var rn pb.Get
	if err := r.ReadMsgWithContext(ctx, &rn); err != nil {
		return fmt.Errorf("read get range: %w", err)
	}

	// recreate the reader to allow the first one to be garbage collected
	// before the makeOffer function call, to reduce the total memory allocated
	// while makeOffer is executing (waiting for the new chunks)
	w, r := protobuf.NewWriterAndReader(stream)

	// make an offer to the upstream peer in return for the requested range
	offer, err := s.makeOffer(ctx, rn)
	if err != nil {
		return fmt.Errorf("make offer: %w", err)
	}

	if err := w.WriteMsgWithContext(ctx, offer); err != nil {
		return fmt.Errorf("write offer: %w", err)
	}

	// we don't have any hashes to offer in this range (the
	// interval is empty). nothing more to do
	if len(offer.Chunks) == 0 {
		return nil
	}

	var want pb.Want
	if err := r.ReadMsgWithContext(ctx, &want); err != nil {
		return fmt.Errorf("read want: %w", err)
	}

	chs, err := s.processWant(ctx, offer, &want)
	if err != nil {
		return fmt.Errorf("process want: %w", err)
	}

	// slow down future requests
	waitDur, err := s.limiter.Wait(streamCtx, p.Address.ByteString(), max(1, len(chs)))
	if err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}
	if waitDur > 0 {
		s.logger.Debug("rate limited peer", "wait_duration", waitDur, "peer_address", p.Address)
	}

	for _, c := range chs {
		var stamp []byte
		if c.Stamp() != nil {
			stamp, err = c.Stamp().MarshalBinary()
			if err != nil {
				return fmt.Errorf("serialise stamp: %w", err)
			}
		}

		deliver := pb.Delivery{Address: c.Address().Bytes(), Data: c.Data(), Stamp: stamp}
		if err := w.WriteMsgWithContext(ctx, &deliver); err != nil {
			return fmt.Errorf("write delivery: %w", err)
		}
	}

	return nil
}
