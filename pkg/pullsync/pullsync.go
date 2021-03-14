// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/bitvector"
	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pullsync/pb"
	"github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName     = "pullsync"
	protocolVersion  = "1.0.0"
	streamName       = "pullsync"
	cursorStreamName = "cursors"
	cancelStreamName = "cancel"
)

var (
	ErrUnsolicitedChunk = errors.New("peer sent unsolicited chunk")

	cancellationTimeout = 5 * time.Second // explicit ruid cancellation message timeout
)

// how many maximum chunks in a batch
var maxPage = 50

// Interface is the PullSync interface.
type Interface interface {
	// SyncInterval syncs a requested interval from the given peer.
	// It returns the BinID of highest chunk that was synced from the given
	// interval. If the requested interval is too large, the downstream peer
	// has the liberty to provide less chunks than requested.
	SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from, to uint64) (topmost uint64, ruid uint32, err error)
	// GetCursors retrieves all cursors from a downstream peer.
	GetCursors(ctx context.Context, peer swarm.Address) ([]uint64, error)
	// CancelRuid cancels active pullsync operation identified by ruid on
	// a downstream peer.
	CancelRuid(ctx context.Context, peer swarm.Address, ruid uint32) error
}

type Syncer struct {
	streamer         p2p.Streamer
	metrics          metrics
	logger           logging.Logger
	storage          pullstorage.Storer
	quit             chan struct{}
	wg               sync.WaitGroup
	unwrap           func(swarm.Chunk)
	attachValidStamp func(swarm.Chunk, []byte) error

	ruidMtx sync.Mutex
	ruidCtx map[uint32]func()

	Interface
	io.Closer
}

func New(streamer p2p.Streamer, storage pullstorage.Storer, unwrap func(swarm.Chunk), attachValidStamp func(swarm.Chunk, []byte) error, logger logging.Logger) *Syncer {
	return &Syncer{
		streamer:         streamer,
		storage:          storage,
		metrics:          newMetrics(),
		unwrap:           unwrap,
		attachValidStamp: attachValidStamp,
		logger:           logger,
		ruidCtx:          make(map[uint32]func()),
		wg:               sync.WaitGroup{},
		quit:             make(chan struct{}),
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
			{
				Name:    cancelStreamName,
				Handler: s.cancelHandler,
			},
		},
	}
}

// SyncInterval syncs a requested interval from the given peer.
// It returns the BinID of highest chunk that was synced from the given interval.
// If the requested interval is too large, the downstream peer has the liberty to
// provide less chunks than requested.
func (s *Syncer) SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from, to uint64) (topmost uint64, ruid uint32, err error) {
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return 0, 0, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	var ru pb.Ruid
	b := make([]byte, 4)
	_, err = rand.Read(b)
	if err != nil {
		return 0, 0, fmt.Errorf("crypto rand: %w", err)
	}

	ru.Ruid = binary.BigEndian.Uint32(b)

	w, r := protobuf.NewWriterAndReader(stream)

	if err = w.WriteMsgWithContext(ctx, &ru); err != nil {
		return 0, 0, fmt.Errorf("write ruid: %w", err)
	}

	rangeMsg := &pb.GetRange{Bin: int32(bin), From: from, To: to}
	if err = w.WriteMsgWithContext(ctx, rangeMsg); err != nil {
		return 0, ru.Ruid, fmt.Errorf("write get range: %w", err)
	}

	var offer pb.Offer
	if err = r.ReadMsgWithContext(ctx, &offer); err != nil {
		return 0, ru.Ruid, fmt.Errorf("read offer: %w", err)
	}

	if len(offer.Hashes)%swarm.HashSize != 0 {
		return 0, ru.Ruid, fmt.Errorf("inconsistent hash length")
	}

	// empty interval (no chunks present in interval).
	// return the end of the requested range as topmost.
	if len(offer.Hashes) == 0 {
		return offer.Topmost, ru.Ruid, nil
	}

	var (
		bvLen      = len(offer.Hashes) / swarm.HashSize
		wantChunks = make(map[string]struct{})
		ctr        = 0
	)

	bv, err := bitvector.New(bvLen)
	if err != nil {
		return 0, ru.Ruid, fmt.Errorf("new bitvector: %w", err)
	}

	for i := 0; i < len(offer.Hashes); i += swarm.HashSize {
		a := swarm.NewAddress(offer.Hashes[i : i+swarm.HashSize])
		if a.Equal(swarm.ZeroAddress) {
			// i'd like to have this around to see we don't see any of these in the logs
			s.logger.Errorf("syncer got a zero address hash on offer")
			return 0, ru.Ruid, fmt.Errorf("zero address on offer")
		}
		s.metrics.OfferCounter.Inc()
		s.metrics.DbOpsCounter.Inc()
		have, err := s.storage.Has(ctx, a)
		if err != nil {
			return 0, ru.Ruid, fmt.Errorf("storage has: %w", err)
		}
		if !have {
			wantChunks[a.String()] = struct{}{}
			ctr++
			s.metrics.WantCounter.Inc()
			bv.Set(i / swarm.HashSize)
		}
	}

	wantMsg := &pb.Want{BitVector: bv.Bytes()}
	if err = w.WriteMsgWithContext(ctx, wantMsg); err != nil {
		return 0, ru.Ruid, fmt.Errorf("write want: %w", err)
	}

	// if ctr is zero, it means we don't want any chunk in the batch
	// thus, the following loop will not get executed and the method
	// returns immediately with the topmost value on the offer, which
	// will seal the interval and request the next one

	for ; ctr > 0; ctr-- {
		var delivery pb.Delivery
		if err = r.ReadMsgWithContext(ctx, &delivery); err != nil {
			return 0, ru.Ruid, fmt.Errorf("read delivery: %w", err)
		}

		addr := swarm.NewAddress(delivery.Address)
		if _, ok := wantChunks[addr.String()]; !ok {
			return 0, ru.Ruid, ErrUnsolicitedChunk
		}

		delete(wantChunks, addr.String())
		s.metrics.DbOpsCounter.Inc()
		s.metrics.DeliveryCounter.Inc()

		chunk := swarm.NewChunk(addr, delivery.Data)
		if err = s.attachValidStamp(chunk, delivery.Stamp); err != nil {
			return 0, ru.Ruid, err
		}
		if content.Valid(chunk) {
			go s.unwrap(chunk)
		} else if !soc.Valid(chunk) {
			return 0, ru.Ruid, swarm.ErrInvalidChunk
		}
		if err = s.storage.Put(ctx, storage.ModePutSync, chunk); err != nil {
			return 0, ru.Ruid, fmt.Errorf("delivery put: %w", err)
		}
	}
	return offer.Topmost, ru.Ruid, nil
}

// handler handles an incoming request to sync an interval
func (s *Syncer) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	var ru pb.Ruid
	if err := r.ReadMsgWithContext(ctx, &ru); err != nil {
		return fmt.Errorf("send ruid: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s.ruidMtx.Lock()
	s.ruidCtx[ru.Ruid] = cancel
	s.ruidMtx.Unlock()
	cc := make(chan struct{})
	defer close(cc)
	go func() {
		select {
		case <-s.quit:
		case <-ctx.Done():
		case <-cc:
		}
		cancel()
		s.ruidMtx.Lock()
		delete(s.ruidCtx, ru.Ruid)
		s.ruidMtx.Unlock()
	}()

	select {
	case <-s.quit:
		return nil
	default:
	}

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

	time.Sleep(50 * time.Millisecond) //because of test, getting EOF w/o
	return nil
}

// makeOffer tries to assemble an offer for a given requested interval.
func (s *Syncer) makeOffer(ctx context.Context, rn pb.GetRange) (o *pb.Offer, addrs []swarm.Address, err error) {
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
	s.metrics.DbOpsCounter.Inc()
	return s.storage.Get(ctx, storage.ModeGetSync, addrs...)
}

func (s *Syncer) GetCursors(ctx context.Context, peer swarm.Address) (retr []uint64, err error) {
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, cursorStreamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
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
	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	var syn pb.Syn
	if err := r.ReadMsgWithContext(ctx, &syn); err != nil {
		return fmt.Errorf("read syn: %w", err)
	}

	var ack pb.Ack
	s.metrics.DbOpsCounter.Inc()
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

func (s *Syncer) CancelRuid(ctx context.Context, peer swarm.Address, ruid uint32) (err error) {
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, cancelStreamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}

	w := protobuf.NewWriter(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, cancellationTimeout)
	defer cancel()

	var c pb.Cancel
	c.Ruid = ruid
	if err := w.WriteMsgWithContext(ctx, &c); err != nil {
		return fmt.Errorf("send cancellation: %w", err)
	}
	return nil
}

// handler handles an incoming request to explicitly cancel a ruid
func (s *Syncer) cancelHandler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	r := protobuf.NewReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	var c pb.Cancel
	if err := r.ReadMsgWithContext(ctx, &c); err != nil {
		return fmt.Errorf("read cancel: %w", err)
	}

	s.ruidMtx.Lock()
	defer s.ruidMtx.Unlock()

	if cancel, ok := s.ruidCtx[c.Ruid]; ok {
		cancel()
	}
	delete(s.ruidCtx, c.Ruid)
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
	case <-time.After(10 * time.Second):
		s.logger.Warning("pull syncer shutting down with running goroutines")
	}
	return nil
}
