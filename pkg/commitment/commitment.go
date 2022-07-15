// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pullsync provides the pullsync protocol
// implementation.
package commitment

import (
	// "context"
	"crypto/hmac"
	// "encoding/binary"
	"errors"
	// "fmt"
	// "io"
	// "sync"
	// "time"

	// "github.com/ethersphere/bee/pkg/bitvector"
	// "github.com/ethersphere/bee/pkg/cac"
	// "github.com/ethersphere/bee/pkg/logging"
	// "github.com/ethersphere/bee/pkg/p2p"
	// "github.com/ethersphere/bee/pkg/p2p/protobuf"
	// "github.com/ethersphere/bee/pkg/postage"
	// "github.com/ethersphere/bee/pkg/pullsync/pb"
	// "github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	// "github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	// "github.com/ethersphere/bee/pkg/swarm"
)

const ()

const logMore = false // enable this for more logging

var (
	ErrUnsolicitedChunk = errors.New("peer sent unsolicited chunk")
	maxlimit            = 5000000
)

const ()

// Interface is the PullSync interface.
type Interface interface {
}

type Sampler struct {
	//	metrics    metrics
	logger  logging.Logger
	storage storage.Storer
	//	quit       chan struct{}
	//	wg         sync.WaitGroup
	//
	//	Interface
	//	io.Closer
}

func New(storage storage.Storer, logger logging.Logger) *Sampler {
	return &Sampler{
		storage: storage,
		// metrics:    newMetrics(),
		// unwrap:     unwrap,
		// validStamp: validStamp,
		logger: logger,
		// ruidCtx:    make(map[string]map[uint32]func()),
		// wg:         sync.WaitGroup{},
		// quit:       make(chan struct{}),
	}
}

type SampleItem struct {
	ChunkAddress            swarm.Address
	TransformedChunkAddress swarm.Address
}

type ReserveCommitment struct {
	Items []SampleItem
	Hash  swarm.Address
}

// makeOffer tries to assemble an offer for a given requested interval.
func (s *Syncer) MakeSample(ctx context.Context, anchor []byte, storageDepth uint8) (taddrs []SampleItem, err error) {

	hmacr := hmac.New(swarm.NewHasher(), anchor)

	zerobytes := make([]bytes, 32)
	buffer := make([]SampleItem)

	for bin := storageDepth; bin <= swarm.MaxPO; bin++ {
		chs, top, err := s.storage.SubscribePull(ctx, bin, 0, 0, maxlimit)
		if err != nil {
			return o, nil, err
		}

		for ch := range chs {
			chunk, err := s.storage.Get(ctx, storage.ModeGetRequest, ch)
			if err != nil {
				s.logger.Error("reserve sampler: skipping missing chunk")
				continue
			}
			hmacr.Write(chunk.Data())
			taddr := hmacr.Sum(nil)
			hmacr.Reset()

			for i, item := range buffer {
				distance, err := swarm.DistanceCmp(zerobytes, taddr, item.TransformedChunkAddress.Bytes())
				if err != nil {
					break
				}
				if distance > 0 {
					buffer = append(buffer[:i+1], buffer[i:]...)
					buffer[i] = SampleItem{ChunkAddress: ch, TransformedChunkAddress: swarm.NewAddress(taddr)}
					break
				}
				if distance == 0 {
					break
				}
			}

			if len(buffer) > 16 {
				buffer = buffer[:16]
			}

			if len(buffer) < 16 {
				buffer[len(buffer)] = SampleItem{ChunkAddress: ch, TransformedChunkAddress: swarm.NewAddress(taddr)}
			}

		}

	}

	if len(buffer) < 16 {
		return nil, error
	}
	hasher := swarm.NewHasher()

	for item := range buffer {
		hasher.Write(item.TransformedChunkAddress.Bytes())
	}

	hash := hasher.Sum(nil)

	return ReserveCommitment{Hash: hash, Items: buffer}, nil

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
