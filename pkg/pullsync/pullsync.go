//go:build !js
// +build !js

package pullsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/bitvector"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/ratelimit"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"resenje.org/singleflight"
)

type Syncer struct {
	streamer       p2p.Streamer
	metrics        metrics
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
		metrics:     newMetrics(),
		unwrap:      unwrap,
		gsocHandler: gsocHandler,
		validStamp:  validStamp,
		logger:      logger.WithName(loggerName).Register(),
		quit:        make(chan struct{}),
		maxPage:     maxPage,
		limiter:     ratelimit.New(handleRequestsLimitRate, int(maxPage)),
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

	s.metrics.SentOffered.Add(float64(len(offer.Chunks)))

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
		s.metrics.Sent.Inc()
	}

	return nil
}

// Sync syncs a batch of chunks starting at a start BinID.
// It returns the BinID of highest chunk that was synced from the given
// batch and the total number of chunks the downstream peer has sent.
func (s *Syncer) Sync(ctx context.Context, peer swarm.Address, bin uint8, start uint64) (uint64, int, error) {

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return 0, 0, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
			s.logger.Debug("error syncing peer", "peer_address", peer, "bin", bin, "start", start, "error", err)
		} else {
			stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)

	rangeMsg := &pb.Get{Bin: int32(bin), Start: start}
	if err = w.WriteMsgWithContext(ctx, rangeMsg); err != nil {
		return 0, 0, fmt.Errorf("write get range: %w", err)
	}

	var offer pb.Offer
	if err = r.ReadMsgWithContext(ctx, &offer); err != nil {
		return 0, 0, fmt.Errorf("read offer: %w", err)
	}

	// empty interval (no chunks present in interval).
	// return the end of the requested range as topmost.
	if len(offer.Chunks) == 0 {
		return offer.Topmost, 0, nil
	}

	topmost := offer.Topmost

	var (
		bvLen      = len(offer.Chunks)
		wantChunks = make(map[string]struct{}, bvLen)
		ctr        = 0
		have       bool
	)

	bv, err := bitvector.New(bvLen)
	if err != nil {
		return 0, 0, fmt.Errorf("new bitvector: %w", err)
	}

	for i := 0; i < len(offer.Chunks); i++ {

		addr := offer.Chunks[i].Address
		batchID := offer.Chunks[i].BatchID
		stampHash := offer.Chunks[i].StampHash
		if len(addr) != swarm.HashSize {
			return 0, 0, fmt.Errorf("inconsistent hash length")
		}

		a := swarm.NewAddress(addr)
		if a.Equal(swarm.ZeroAddress) {
			// i'd like to have this around to see we don't see any of these in the logs
			s.logger.Debug("syncer got a zero address hash on offer", "peer_address", peer)
			continue
		}
		s.metrics.Offered.Inc()
		if s.store.IsWithinStorageRadius(a) {
			have, err = s.store.ReserveHas(a, batchID, stampHash)
			if err != nil {
				s.logger.Debug("storage has", "error", err)
				return 0, 0, err
			}

			if !have {
				wantChunks[a.ByteString()+string(batchID)+string(stampHash)] = struct{}{}
				ctr++
				s.metrics.Wanted.Inc()
				bv.Set(i)
			}
		}
	}

	wantMsg := &pb.Want{BitVector: bv.Bytes()}
	if err = w.WriteMsgWithContext(ctx, wantMsg); err != nil {
		return 0, 0, fmt.Errorf("write want: %w", err)
	}

	chunksToPut := make([]swarm.Chunk, 0, ctr)

	var chunkErr error
	for ; ctr > 0; ctr-- {
		var delivery pb.Delivery
		if err = r.ReadMsgWithContext(ctx, &delivery); err != nil {
			return 0, 0, errors.Join(chunkErr, fmt.Errorf("read delivery: %w", err))
		}

		addr := swarm.NewAddress(delivery.Address)
		if addr.Equal(swarm.ZeroAddress) {
			s.logger.Debug("received zero address chunk", "peer_address", peer)
			s.metrics.ReceivedZeroAddress.Inc()
			continue
		}

		newChunk := swarm.NewChunk(addr, delivery.Data)

		stamp := new(postage.Stamp)
		if err = stamp.UnmarshalBinary(delivery.Stamp); err != nil {
			chunkErr = errors.Join(chunkErr, err)
			continue
		}
		stampHash, err := stamp.Hash()
		if err != nil {
			chunkErr = errors.Join(chunkErr, err)
			continue
		}

		wantChunkID := addr.ByteString() + string(stamp.BatchID()) + string(stampHash)
		if _, ok := wantChunks[wantChunkID]; !ok {
			s.logger.Debug("want chunks", "error", ErrUnsolicitedChunk, "peer_address", peer, "chunk_address", addr)
			chunkErr = errors.Join(chunkErr, ErrUnsolicitedChunk)
			continue
		}

		delete(wantChunks, wantChunkID)

		chunk, err := s.validStamp(newChunk.WithStamp(stamp))
		if err != nil {
			s.logger.Debug("unverified stamp", "error", err, "peer_address", peer, "chunk_address", newChunk)
			chunkErr = errors.Join(chunkErr, err)
			continue
		}

		if cac.Valid(chunk) {
			go s.unwrap(chunk)
		} else if chunk, err := soc.FromChunk(chunk); err == nil {
			addr, err := chunk.Address()
			if err != nil {
				chunkErr = errors.Join(chunkErr, err)
				continue
			}
			s.logger.Debug("sync gsoc", "peer_address", peer, "chunk_address", addr, "wrapped_chunk_address", chunk.WrappedChunk().Address())
			s.gsocHandler(chunk)
		} else {
			s.logger.Debug("invalid cac/soc chunk", "error", swarm.ErrInvalidChunk, "peer_address", peer, "chunk", chunk)
			chunkErr = errors.Join(chunkErr, swarm.ErrInvalidChunk)
			s.metrics.ReceivedInvalidChunk.Inc()
			continue
		}
		chunksToPut = append(chunksToPut, chunk)
	}

	chunksPut := 0
	if len(chunksToPut) > 0 {

		s.metrics.Delivered.Add(float64(len(chunksToPut)))
		s.metrics.LastReceived.WithLabelValues(fmt.Sprintf("%d", bin)).Add(float64(len(chunksToPut)))

		for _, c := range chunksToPut {
			if err := s.store.ReservePutter().Put(ctx, c); err != nil {
				// in case of these errors, no new items are added to the storage, so it
				// is safe to continue with the next chunk
				if errors.Is(err, storage.ErrOverwriteNewerChunk) {
					s.logger.Debug("overwrite newer chunk", "error", err, "peer_address", peer, "chunk", c)
					chunkErr = errors.Join(chunkErr, err)
					continue
				}
				return 0, 0, errors.Join(chunkErr, err)
			}
			chunksPut++
		}
	}

	return topmost, chunksPut, chunkErr
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
