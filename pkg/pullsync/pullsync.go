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
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/bitvector"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pullsync/pb"
	"github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	"github.com/ethersphere/bee/pkg/rate"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pullsync"

const (
	protocolName     = "pullsync"
	protocolVersion  = "1.2.0"
	streamName       = "pullsync"
	cursorStreamName = "cursors"
	cancelStreamName = "cancel"
)

const (
	MaxCursor           = math.MaxUint64
	DefaultRateDuration = time.Minute * 15
)

var (
	ErrUnsolicitedChunk = errors.New("peer sent unsolicited chunk")
)

const (
	storagePutTimeout = time.Second * 15
	makeOfferTimeout  = time.Minute * 15
)

// how many maximum chunks in a batch
var maxPage = 250

// Interface is the PullSync interface.
type Interface interface {
	// SyncInterval syncs a requested interval from the given peer.
	// It returns the BinID of highest chunk that was synced from the given
	// interval. If the requested interval is too large, the downstream peer
	// has the liberty to provide less chunks than requested.
	// A zero topmost is returned if a catastrophic error was encountered and the same interval should be tried again.
	SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from, to uint64) (topmost uint64, err error)
	// GetCursors retrieves all cursors from a downstream peer.
	GetCursors(ctx context.Context, peer swarm.Address) ([]uint64, error)
}

type SyncReporter interface {
	// Number of active historical syncing jobs.
	Rate() float64
}

type Syncer struct {
	streamer       p2p.Streamer
	metrics        metrics
	logger         log.Logger
	storage        pullstorage.Storer
	quit           chan struct{}
	wg             sync.WaitGroup
	unwrap         func(swarm.Chunk)
	validStamp     postage.ValidStampFn
	radius         postage.Radius
	overlayAddress swarm.Address

	rate *rate.Rate

	Interface
	io.Closer
}

func New(streamer p2p.Streamer, storage pullstorage.Storer, unwrap func(swarm.Chunk), validStamp postage.ValidStampFn, logger log.Logger, radius postage.Radius, overlayAddress swarm.Address) *Syncer {

	return &Syncer{
		streamer:       streamer,
		storage:        storage,
		metrics:        newMetrics(),
		unwrap:         unwrap,
		validStamp:     validStamp,
		logger:         logger.WithName(loggerName).Register(),
		wg:             sync.WaitGroup{},
		quit:           make(chan struct{}),
		radius:         radius,
		overlayAddress: overlayAddress,
		rate:           rate.New(DefaultRateDuration),
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

// SyncInterval syncs a requested interval from the given peer.
// It returns the BinID of the highest chunk that was synced from the given interval.
// If the requested interval is too large, the downstream peer has the liberty to
// provide fewer chunks than requested.
func (s *Syncer) SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from, to uint64) (uint64, error) {
	loggerV2 := s.logger.V(2).Register()

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return 0, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
			loggerV2.Debug("error syncing peer", "peer_address", peer, "bin", bin, "from", from, "to", to, "error", err)
		} else {
			stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)

	rangeMsg := &pb.GetRange{Bin: int32(bin), From: from, To: to}
	if err = w.WriteMsgWithContext(ctx, rangeMsg); err != nil {
		return 0, fmt.Errorf("write get range: %w", err)
	}

	var offer pb.Offer
	if err = r.ReadMsgWithContext(ctx, &offer); err != nil {
		return 0, fmt.Errorf("read offer: %w", err)
	}

	if len(offer.Hashes)%swarm.HashSize != 0 {
		return 0, fmt.Errorf("inconsistent hash length")
	}

	// empty interval (no chunks present in interval).
	// return the end of the requested range as topmost.
	if len(offer.Hashes) == 0 {
		return offer.Topmost, nil
	}

	topmost := offer.Topmost

	var (
		bvLen      = len(offer.Hashes) / swarm.HashSize
		wantChunks = make(map[string]struct{})
		ctr        = 0
		have       bool
	)

	bv, err := bitvector.New(bvLen)
	if err != nil {
		return 0, fmt.Errorf("new bitvector: %w", err)
	}

	for i := 0; i < len(offer.Hashes); i += swarm.HashSize {
		a := swarm.NewAddress(offer.Hashes[i : i+swarm.HashSize])
		if a.Equal(swarm.ZeroAddress) {
			// i'd like to have this around to see we don't see any of these in the logs
			s.logger.Debug("syncer got a zero address hash on offer", "peer_address", peer)
			continue
		}
		s.metrics.Offered.Inc()
		s.metrics.DbOps.Inc()
		po := swarm.Proximity(a.Bytes(), s.overlayAddress.Bytes())
		if po >= s.radius.StorageRadius() {
			have, err = s.storage.Has(ctx, a)
			if err != nil {
				s.logger.Debug("storage has", "error", err)
				continue
			}

			if !have {
				wantChunks[a.ByteString()] = struct{}{}
				ctr++
				s.metrics.Wanted.Inc()
				bv.Set(i / swarm.HashSize)
			}
		}
	}

	wantMsg := &pb.Want{BitVector: bv.Bytes()}
	if err = w.WriteMsgWithContext(ctx, wantMsg); err != nil {
		return 0, fmt.Errorf("write want: %w", err)
	}

	var chunksToPut []swarm.Chunk

	var chunkErr error
	for ; ctr > 0; ctr-- {
		var delivery pb.Delivery
		if err = r.ReadMsgWithContext(ctx, &delivery); err != nil {
			return 0, fmt.Errorf("read delivery: %w", err)
		}

		addr := swarm.NewAddress(delivery.Address)
		if _, ok := wantChunks[addr.ByteString()]; !ok {
			loggerV2.Debug("want chunks", "error", ErrUnsolicitedChunk, "peer_address", peer, "chunk_address", addr)
			chunkErr = errors.Join(chunkErr, ErrUnsolicitedChunk)
			continue
		}

		delete(wantChunks, addr.ByteString())
		s.metrics.Delivered.Inc()

		newChunk := swarm.NewChunk(addr, delivery.Data)
		chunk, err := s.validStamp(newChunk, delivery.Stamp)
		if err != nil {
			loggerV2.Debug("unverified stamp", "error", err, "peer_address", peer, "chunk_address", newChunk)
			chunkErr = errors.Join(chunkErr, err)
			continue
		}

		if cac.Valid(chunk) {
			go s.unwrap(chunk)
		} else if !soc.Valid(chunk) {
			loggerV2.Debug("valid chunk", "error", swarm.ErrInvalidChunk, "peer_address", peer, "chunk_address", chunk)
			chunkErr = errors.Join(chunkErr, swarm.ErrInvalidChunk)
			continue
		}
		chunksToPut = append(chunksToPut, chunk)
	}

	if len(chunksToPut) > 0 {

		if to != MaxCursor { // historical syncing
			s.rate.Add(len(chunksToPut))
		}

		s.metrics.DbOps.Inc()

		if err := s.storage.Put(ctx, storage.ModePutSync, chunksToPut...); err != nil {
			return 0, errors.Join(chunkErr, fmt.Errorf("delivery put: %w", err))
		}
		s.metrics.LastReceived.WithLabelValues(fmt.Sprintf("%d", bin)).Set(float64(time.Now().Unix()))
	}

	return topmost, chunkErr
}

// SyncRate returns chunks per second synced.
func (s *Syncer) SyncRate() float64 {
	return s.rate.Rate()
}

// handler handles an incoming request to sync an interval
func (s *Syncer) handler(streamCtx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	select {
	case <-s.quit:
		return nil
	default:
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

	s.wg.Add(1)
	defer s.wg.Done()

	var rn pb.GetRange
	if err := r.ReadMsgWithContext(ctx, &rn); err != nil {
		return fmt.Errorf("read get range: %w", err)
	}

	// make an offer to the upstream peer in return for the requested range
	offer, _, err := s.makeOffer(ctx, rn)
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
	if len(offer.Hashes) == 0 {
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

	for _, v := range chs {
		stamp, err := v.Stamp().MarshalBinary()
		if err != nil {
			return fmt.Errorf("serialise stamp: %w", err)
		}
		deliver := pb.Delivery{Address: v.Address().Bytes(), Data: v.Data(), Stamp: stamp}
		if err := w.WriteMsgWithContext(ctx, &deliver); err != nil {
			return fmt.Errorf("write delivery: %w", err)
		}
	}

	return nil
}

// makeOffer tries to assemble an offer for a given requested interval.
func (s *Syncer) makeOffer(ctx context.Context, rn pb.GetRange) (o *pb.Offer, addrs []swarm.Address, err error) {

	ctx, cancel := context.WithTimeout(ctx, makeOfferTimeout)
	defer cancel()

	chs, top, err := s.storage.IntervalChunks(ctx, uint8(rn.Bin), rn.From, rn.To, maxPage)
	if err != nil {
		return o, nil, err
	}
	o = new(pb.Offer)
	o.Topmost = top
	o.Hashes = make([]byte, 0)
	for _, v := range chs {
		o.Hashes = append(o.Hashes, v.Bytes()...)
	}
	return o, chs, nil
}

// processWant compares a received Want to a sent Offer and returns
// the appropriate chunks from the local store.
func (s *Syncer) processWant(ctx context.Context, o *pb.Offer, w *pb.Want) ([]swarm.Chunk, error) {
	l := len(o.Hashes) / swarm.HashSize
	bv, err := bitvector.NewFromBytes(w.BitVector, l)
	if err != nil {
		return nil, err
	}

	var addrs []swarm.Address
	for i := 0; i < len(o.Hashes); i += swarm.HashSize {
		if bv.Get(i / swarm.HashSize) {
			a := swarm.NewAddress(o.Hashes[i : i+swarm.HashSize])
			addrs = append(addrs, a)
		}
	}
	s.metrics.DbOps.Inc()
	return s.storage.Get(ctx, storage.ModeGetSync, addrs...)
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
	s.metrics.DbOps.Inc()
	ints, err := s.storage.Cursors(ctx)
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
		s.wg.Wait()
	}()

	select {
	case <-cc:
	case <-time.After(5 * time.Second):
		s.logger.Warning("pull syncer shutting down with running goroutines")
	}
	return nil
}
