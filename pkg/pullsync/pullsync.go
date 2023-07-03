// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pullsync provides the pullsync protocol
// implementation.
package pullsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/bitvector"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pullsync/pb"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"resenje.org/singleflight"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pullsync"

const (
	protocolName     = "pullsync"
	protocolVersion  = "1.3.0"
	streamName       = "pullsync"
	cursorStreamName = "cursors"
	cancelStreamName = "cancel"
)

const (
	MaxCursor           = math.MaxUint64
	DefaultRateDuration = time.Minute * 15
	batchTimeout        = time.Second
)

var (
	ErrUnsolicitedChunk = errors.New("peer sent unsolicited chunk")
)

const (
	makeOfferTimeout        = 5 * time.Minute
	DefaultMaxPage   uint64 = 250
)

// singleflight key for intervals
func sfKey(bin uint8, start uint64) string {
	return fmt.Sprintf("%d-%d", bin, start)
}

// how many maximum chunks in a batch

// Interface is the PullSync interface.
type Interface interface {
	// Sync syncs a batch of chunks starting at a start BinID.
	// It returns the BinID of highest chunk that was synced from the given
	// batch and the total number of chunks the downstream peer has sent.
	Sync(ctx context.Context, peer swarm.Address, bin uint8, start uint64) (topmost uint64, count int, err error)
	// GetCursors retrieves all cursors from a downstream peer.
	GetCursors(ctx context.Context, peer swarm.Address) ([]uint64, error)
}

type Syncer struct {
	streamer       p2p.Streamer
	metrics        metrics
	logger         log.Logger
	store          storer.Reserve
	quit           chan struct{}
	unwrap         func(swarm.Chunk)
	validStamp     postage.ValidStampFn
	intervalsSF    singleflight.Group
	syncInProgress atomic.Int32

	maxPage uint64

	Interface
	io.Closer
}

func New(
	streamer p2p.Streamer,
	store storer.Reserve,
	unwrap func(swarm.Chunk),
	validStamp postage.ValidStampFn,
	logger log.Logger,
	maxPage uint64,
) *Syncer {

	return &Syncer{
		streamer:   streamer,
		store:      store,
		metrics:    newMetrics(),
		unwrap:     unwrap,
		validStamp: validStamp,
		logger:     logger.WithName(loggerName).Register(),
		quit:       make(chan struct{}),
		maxPage:    maxPage,
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
	}
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
			have, err = s.store.ReserveHas(a, batchID)
			if err != nil {
				s.logger.Debug("storage has", "error", err)
				continue
			}

			if !have {
				wantChunks[a.ByteString()+string(batchID)] = struct{}{}
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
		newChunk := swarm.NewChunk(addr, delivery.Data)

		stamp := new(postage.Stamp)
		if err = stamp.UnmarshalBinary(delivery.Stamp); err != nil {
			chunkErr = errors.Join(chunkErr, err)
			continue
		}

		if _, ok := wantChunks[addr.ByteString()+string(stamp.BatchID())]; !ok {
			s.logger.Debug("want chunks", "error", ErrUnsolicitedChunk, "peer_address", peer, "chunk_address", addr)
			chunkErr = errors.Join(chunkErr, ErrUnsolicitedChunk)
			continue
		}

		delete(wantChunks, addr.ByteString()+string(stamp.BatchID()))

		chunk, err := s.validStamp(newChunk.WithStamp(stamp))
		if err != nil {
			s.logger.Debug("unverified stamp", "error", err, "peer_address", peer, "chunk_address", newChunk)
			chunkErr = errors.Join(chunkErr, err)
			continue
		}

		if cac.Valid(chunk) {
			go s.unwrap(chunk)
		} else if !soc.Valid(chunk) {
			s.logger.Debug("invalid soc chunk", "error", swarm.ErrInvalidChunk, "peer_address", peer, "chunk", chunk)
			chunkErr = errors.Join(chunkErr, swarm.ErrInvalidChunk)
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
				if errors.Is(err, storage.ErrOverwriteNewerChunk) || errors.Is(err, storage.ErrOverwriteOfImmutableBatch) {
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

	// make an offer to the upstream peer in return for the requested range
	offer, err := s.makeOffer(ctx, rn)
	if err != nil {
		return fmt.Errorf("make offer: %w", err)
	}

	// recreate the reader to allow the first one to be garbage collected
	// before the makeOffer function call, to reduce the total memory allocated
	// while makeOffer is executing (waiting for the new chunks)
	w, r := protobuf.NewWriterAndReader(stream)

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
		if errors.Is(err, storage.ErrNotFound) {
			s.intervalsSF.Forget(sfKey(uint8(rn.Bin), rn.Start))
		}
		return fmt.Errorf("process want: %w", err)
	}

	for _, v := range chs {
		stamp, err := v.Stamp().MarshalBinary()
		if err != nil {
			return fmt.Errorf("serialise stamp: %w", err)
		}
		deliver := pb.Delivery{Address: v.Address().Bytes(), Data: v.Data(), Stamp: stamp}
		if err := w.WriteMsgWithContext(ctx, &deliver); err != nil {
			return fmt.Errorf("write delivery: %w", err)
		}
		s.metrics.Sent.Inc()
	}

	return nil
}

// makeOffer tries to assemble an offer for a given requested interval.
func (s *Syncer) makeOffer(ctx context.Context, rn pb.Get) (*pb.Offer, error) {

	ctx, cancel := context.WithTimeout(ctx, makeOfferTimeout)
	defer cancel()

	addrs, top, err := s.collectAddrs(ctx, uint8(rn.Bin), rn.Start)
	if err != nil {
		return nil, err
	}

	o := new(pb.Offer)
	o.Topmost = top
	o.Chunks = make([]*pb.Chunk, 0, len(addrs))
	for _, v := range addrs {
		o.Chunks = append(o.Chunks, &pb.Chunk{Address: v.Address.Bytes(), BatchID: v.BatchID})
	}
	return o, nil
}

// collectAddrs collects chunk addresses at a bin starting at some start BinID until a limit is reached.
// The function waits for an unbounded amount of time for the first chunk to arrive.
// After the arrival of the first chunk, the subsequent chunks have a limited amount of time to arrive,
// after which the function returns the collected slice of chunks.
func (s *Syncer) collectAddrs(ctx context.Context, bin uint8, start uint64) ([]*storer.BinC, uint64, error) {
	loggerV2 := s.logger.V(2).Register()

	type result struct {
		chs     []*storer.BinC
		topmost uint64
	}

	v, _, err := s.intervalsSF.Do(ctx, sfKey(bin, start), func(ctx context.Context) (interface{}, error) {
		var (
			chs     []*storer.BinC
			topmost uint64
			timer   *time.Timer
			timerC  <-chan time.Time
		)
		chC, unsub, errC := s.store.SubscribeBin(ctx, bin, start)
		defer func() {
			unsub()
			if timer != nil {
				timer.Stop()
			}
		}()

		limit := s.maxPage

	LOOP:
		for limit > 0 {
			select {
			case c, ok := <-chC:
				if !ok {
					break LOOP // The stream has been closed.
				}

				chs = append(chs, &storer.BinC{Address: c.Address, BatchID: c.BatchID})
				if c.BinID > topmost {
					topmost = c.BinID
				}
				limit--
				if timer == nil {
					timer = time.NewTimer(batchTimeout)
				} else {
					if !timer.Stop() {
						<-timer.C
					}
					timer.Reset(batchTimeout)
				}
				timerC = timer.C
			case err := <-errC:
				return nil, err
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timerC:
				loggerV2.Debug("batch timeout timer triggered")
				// return batch if new chunks are not received after some time
				break LOOP
			}
		}

		return &result{chs: chs, topmost: topmost}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	r := v.(*result)
	return r.chs, r.topmost, nil
}

// processWant compares a received Want to a sent Offer and returns
// the appropriate chunks from the local store.
func (s *Syncer) processWant(ctx context.Context, o *pb.Offer, w *pb.Want) ([]swarm.Chunk, error) {
	bv, err := bitvector.NewFromBytes(w.BitVector, len(o.Chunks))
	if err != nil {
		return nil, err
	}

	chunks := make([]swarm.Chunk, 0, len(o.Chunks))
	for i := 0; i < len(o.Chunks); i++ {
		if bv.Get(i) {
			ch := o.Chunks[i]
			c, err := s.store.ReserveGet(ctx, swarm.NewAddress(ch.Address), ch.BatchID)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, c)
		}
	}
	return chunks, nil
}

func (s *Syncer) GetCursors(ctx context.Context, peer swarm.Address) (retr []uint64, err error) {
	loggerV2 := s.logger.V(2).Register()

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, cursorStreamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	loggerV2.Debug("getting cursors from peer", "peer_address", peer)
	defer func() {
		if err != nil {
			_ = stream.Reset()
			loggerV2.Debug("error getting cursors from peer", "peer_address", peer, "error", err)
		} else {
			stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	syn := &pb.Syn{}
	if err = w.WriteMsgWithContext(ctx, syn); err != nil {
		return nil, fmt.Errorf("write syn: %w", err)
	}

	var ack pb.Ack
	if err = r.ReadMsgWithContext(ctx, &ack); err != nil {
		return nil, fmt.Errorf("read ack: %w", err)
	}

	retr = ack.Cursors

	return retr, nil
}

func (s *Syncer) cursorHandler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	loggerV2 := s.logger.V(2).Register()

	w, r := protobuf.NewWriterAndReader(stream)
	loggerV2.Debug("peer wants cursors", "peer_address", p.Address)
	defer func() {
		if err != nil {
			_ = stream.Reset()
			loggerV2.Debug("error getting cursors for peer", "peer_address", p.Address, "error", err)
		} else {
			_ = stream.FullClose()
		}
	}()

	var syn pb.Syn
	if err := r.ReadMsgWithContext(ctx, &syn); err != nil {
		return fmt.Errorf("read syn: %w", err)
	}

	var ack pb.Ack
	ints, err := s.store.ReserveLastBinIDs()
	if err != nil {
		return err
	}
	ack.Cursors = ints
	if err = w.WriteMsgWithContext(ctx, &ack); err != nil {
		return fmt.Errorf("write ack: %w", err)
	}

	return nil
}

func (s *Syncer) Close() error {
	s.logger.Info("pull syncer shutting down")
	close(s.quit)
	cc := make(chan struct{})
	go func() {
		defer close(cc)
		for {
			if s.syncInProgress.Load() > 0 {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			break
		}
	}()

	select {
	case <-cc:
	case <-time.After(5 * time.Second):
		s.logger.Warning("pull syncer shutting down with running goroutines")
	}
	return nil
}
