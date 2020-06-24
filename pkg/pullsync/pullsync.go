// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/pkg/bitvector"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"

	"github.com/ethersphere/bee/pkg/pullsync/pb"
	"github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName     = "pullsync"
	protocolVersion  = "1.0.0"
	streamName       = "pullsync"
	cursorStreamName = "cursors"
)

var (
	ErrUnsolicitedChunk = errors.New("peer sent unsolicited chunk")
)

// how many maximum chunks in a batch
var maxPage = 50

type Interface interface {
	SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from, to uint64) (topmost uint64, err error)
	GetCursors(ctx context.Context, peer swarm.Address) ([]uint64, error)
}

type Syncer struct {
	streamer p2p.Streamer
	logger   logging.Logger
	storage  pullstorage.Storer

	Interface
	io.Closer
}

type Options struct {
	Streamer p2p.Streamer
	Storage  pullstorage.Storer

	Logger logging.Logger
}

func New(o Options) *Syncer {
	return &Syncer{
		streamer: o.Streamer,
		storage:  o.Storage,
		logger:   o.Logger,
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
// It returns the BinID of highest chunk that was synced from the given interval.
// If the requested interval is too large, the downstream peer has the liberty to
// provide less chunks than requested.
func (s *Syncer) SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from, to uint64) (topmost uint64, err error) {
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return 0, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.FullClose()
			return
		}
		_ = stream.Close()
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

	var (
		bvLen      = len(offer.Hashes) / swarm.HashSize
		wantChunks = make(map[string]struct{})
		ctr        = 0
	)

	bv, err := bitvector.New(bvLen)
	if err != nil {
		return 0, fmt.Errorf("new bitvector: %w", err)
	}

	for i := 0; i < len(offer.Hashes); i += swarm.HashSize {
		a := swarm.NewAddress(offer.Hashes[i : i+swarm.HashSize])
		if a.Equal(swarm.ZeroAddress) {
			// i'd like to have this around to see we don't see any of these in the logs
			s.logger.Errorf("syncer got a zero address hash on offer")
			return 0, fmt.Errorf("zero address on offer")
		}
		have, err := s.storage.Has(ctx, a)
		if err != nil {
			return 0, fmt.Errorf("storage has: %w", err)
		}
		if !have {
			wantChunks[a.String()] = struct{}{}
			ctr++
			bv.Set(i / swarm.HashSize)
		}
	}

	wantMsg := &pb.Want{BitVector: bv.Bytes()}
	if err = w.WriteMsgWithContext(ctx, wantMsg); err != nil {
		return 0, fmt.Errorf("write want: %w", err)
	}

	// if ctr is zero, it means we don't want any chunk in the batch
	// thus, the following loop will not get executed and the method
	// returns immediately with the topmost value on the offer, which
	// will seal the interval and request the next one

	for ; ctr > 0; ctr-- {
		var delivery pb.Delivery
		if err = r.ReadMsgWithContext(ctx, &delivery); err != nil {
			return 0, fmt.Errorf("read delivery: %w", err)
		}

		addr := swarm.NewAddress(delivery.Address)
		if _, ok := wantChunks[addr.String()]; !ok {
			return 0, ErrUnsolicitedChunk
		}

		delete(wantChunks, addr.String())

		if err = s.storage.Put(ctx, storage.ModePutSync, swarm.NewChunk(addr, delivery.Data)); err != nil {
			return 0, fmt.Errorf("delivery put: %w", err)
		}
	}
	return offer.Topmost, nil
}

// handler handles an incoming request to sync an interval
func (s *Syncer) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var rn pb.GetRange
	if err := r.ReadMsgWithContext(ctx, &rn); err != nil {
		return fmt.Errorf("read get range: %w", err)
	}

	// make an offer to the upstream peer in return for the requested range
	offer, addrs, err := s.makeOffer(ctx, rn)
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

	// empty bitvector implies downstream peer does not want
	// any chunks (it has them already). mark chunks as synced
	if len(want.BitVector) == 0 {
		return s.setChunks(ctx, addrs...)
	}

	chs, err := s.processWant(ctx, offer, &want)
	if err != nil {
		return fmt.Errorf("process want: %w", err)
	}

	for _, v := range chs {
		deliver := pb.Delivery{Address: v.Address().Bytes(), Data: v.Data()}
		if err := w.WriteMsgWithContext(ctx, &deliver); err != nil {
			return fmt.Errorf("write delivery: %w", err)
		}
	}

	err = s.setChunks(ctx, addrs...)
	if err != nil {
		return err
	}

	time.Sleep(50 * time.Millisecond) //because of test, getting EOF w/o
	return nil
}

func (s *Syncer) setChunks(ctx context.Context, addrs ...swarm.Address) error {
	return s.storage.Set(ctx, storage.ModeSetSyncPull, addrs...)
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
	return s.storage.Get(ctx, storage.ModeGetSync, addrs...)
}

func (s *Syncer) GetCursors(ctx context.Context, peer swarm.Address) ([]uint64, error) {
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, cursorStreamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)
	syn := &pb.Syn{}
	if err = w.WriteMsgWithContext(ctx, syn); err != nil {
		return nil, fmt.Errorf("write syn: %w", err)
	}

	var ack pb.Ack
	if err = r.ReadMsgWithContext(ctx, &ack); err != nil {
		return nil, fmt.Errorf("read ack: %w", err)
	}

	return ack.Cursors, nil
}

func (s *Syncer) cursorHandler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var syn pb.Syn
	if err := r.ReadMsgWithContext(ctx, &syn); err != nil {
		return fmt.Errorf("read syn: %w", err)
	}

	var ack pb.Ack
	ints, err := s.storage.Cursors(ctx)
	if err != nil {
		_ = stream.FullClose()
		return err
	}
	ack.Cursors = ints
	if err = w.WriteMsgWithContext(ctx, &ack); err != nil {
		return fmt.Errorf("write ack: %w", err)
	}

	return nil
}

func (s *Syncer) Close() error {
	return nil
}
