// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pullsync provides the pullsync protocol
// implementation.
package pullsync

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bitvector"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pullsync"

const (
	protocolName     = "pullsync"
	protocolVersion  = "1.4.0"
	streamName       = "pullsync"
	cursorStreamName = "cursors"
)

var (
	ErrUnsolicitedChunk = errors.New("peer sent unsolicited chunk")
)

const (
	MaxCursor                       = math.MaxUint64
	DefaultMaxPage           uint64 = 250
	pageTimeout                     = time.Second
	makeOfferTimeout                = 15 * time.Minute
	handleMaxChunksPerSecond        = 250
	handleRequestsLimitRate         = time.Second / handleMaxChunksPerSecond // handle max `handleMaxChunksPerSecond` chunks per second per peer
)

// Interface is the PullSync interface.
type Interface interface {
	// Sync syncs a batch of chunks starting at a start BinID.
	// It returns the BinID of highest chunk that was synced from the given
	// batch and the total number of chunks the downstream peer has sent.
	Sync(ctx context.Context, peer swarm.Address, bin uint8, start uint64) (topmost uint64, count int, err error)
	// GetCursors retrieves all cursors from a downstream peer.
	GetCursors(ctx context.Context, peer swarm.Address) ([]uint64, uint64, error)
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
		o.Chunks = append(o.Chunks, &pb.Chunk{Address: v.Address.Bytes(), BatchID: v.BatchID, StampHash: v.StampHash})
	}
	return o, nil
}

type collectAddrsResult struct {
	chs     []*storer.BinC
	topmost uint64
}

// collectAddrs collects chunk addresses at a bin starting at some start BinID until a limit is reached.
// The function waits for an unbounded amount of time for the first chunk to arrive.
// After the arrival of the first chunk, the subsequent chunks have a limited amount of time to arrive,
// after which the function returns the collected slice of chunks.
func (s *Syncer) collectAddrs(ctx context.Context, bin uint8, start uint64) ([]*storer.BinC, uint64, error) {
	loggerV2 := s.logger.V(2).Register()

	v, _, err := s.intervalsSF.Do(ctx, sfKey(bin, start), func(ctx context.Context) (*collectAddrsResult, error) {
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

				chs = append(chs, &storer.BinC{Address: c.Address, BatchID: c.BatchID, StampHash: c.StampHash})
				if c.BinID > topmost {
					topmost = c.BinID
				}
				limit--
				if timer == nil {
					timer = time.NewTimer(pageTimeout)
				} else {
					if !timer.Stop() {
						<-timer.C
					}
					timer.Reset(pageTimeout)
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

		return &collectAddrsResult{chs: chs, topmost: topmost}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	return v.chs, v.topmost, nil
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
			addr := swarm.NewAddress(ch.Address)

			c, err := s.store.ReserveGet(ctx, addr, ch.BatchID, ch.StampHash)
			if err != nil {
				s.logger.Debug("processing want: unable to find chunk", "chunk_address", addr, "batch_id", hex.EncodeToString(ch.BatchID))
				chunks = append(chunks, swarm.NewChunk(swarm.ZeroAddress, nil))

				continue
			}
			chunks = append(chunks, c)
		}
	}
	return chunks, nil
}

func (s *Syncer) GetCursors(ctx context.Context, peer swarm.Address) (retr []uint64, epoch uint64, err error) {
	loggerV2 := s.logger.V(2).Register()

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, cursorStreamName)
	if err != nil {
		return nil, 0, fmt.Errorf("new stream: %w", err)
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
		return nil, 0, fmt.Errorf("write syn: %w", err)
	}

	var ack pb.Ack
	if err = r.ReadMsgWithContext(ctx, &ack); err != nil {
		return nil, 0, fmt.Errorf("read ack: %w", err)
	}

	return ack.Cursors, ack.Epoch, nil
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
	ints, epoch, err := s.store.ReserveLastBinIDs()
	if err != nil {
		return err
	}
	ack.Cursors = ints
	ack.Epoch = epoch
	if err = w.WriteMsgWithContext(ctx, &ack); err != nil {
		return fmt.Errorf("write ack: %w", err)
	}

	return nil
}

func (s *Syncer) disconnect(peer p2p.Peer) error {
	s.limiter.Clear(peer.Address.ByteString())
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

// singleflight key for intervals
func sfKey(bin uint8, start uint64) string {
	return fmt.Sprintf("%d-%d", bin, start)
}
