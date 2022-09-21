// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package commitment provides the commitment sampler
// implementation.
package commitment

import (
	"context"
	"crypto/hmac"

	// "encoding/binary"
	"errors"
	// "fmt"
	"io"
	"sync"
	"time"

	// "github.com/ethersphere/bee/pkg/bitvector"
	// "github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	// "github.com/ethersphere/bee/pkg/p2p"
	// "github.com/ethersphere/bee/pkg/p2p/protobuf"
	// "github.com/ethersphere/bee/pkg/postage"
	// "github.com/ethersphere/bee/pkg/pullsync/pb"
	// "github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/depthmonitor"
)

const ()

const logMore = false // enable this for more logging

var (
	ErrUnsolicitedChunk = errors.New("peer sent unsolicited chunk")
	maxlimit            = 5000000
)

type Sampler struct {
	//	metrics    metrics
	logger  log.Logger
	storage storage.Storer
	quit    chan struct{}
	wg      sync.WaitGroup
	//
	//	Interface
	io.Closer
}

func New(storage storage.Storer, logger log.Logger) *Sampler {
	return &Sampler{
		storage: storage,
		// metrics:    newMetrics(),
		// unwrap:     unwrap,
		// validStamp: validStamp,
		logger: logger,
		// ruidCtx:    make(map[string]map[uint32]func()),
		wg:   sync.WaitGroup{},
		quit: make(chan struct{}),
	}
}

type SampleItem struct {
	ChunkAddress            swarm.Address `json:"chunkAddress"`
	TransformedChunkAddress swarm.Address `json:"TransformedChunkAddress"`
}

type ReserveCommitment struct {
	Items []SampleItem  `json:"sampleItems"`
	Hash  swarm.Address `json:"rchash"`
}

// MakeSample tries to assemble a reserve commitment sample from the given depth.
func (s *Sampler) MakeSample(ctx context.Context, anchor []byte, storageDepth uint8) (commitmentHash ReserveCommitment, iter, doub, errd, rsvsize uint64, err error) {

	added := false

	iterated := uint64(0)
	errored := uint64(0)
	doubled := uint64(0)

	hmacr := hmac.New(swarm.NewHasher, anchor)

	zerobytes := make([]byte, 32)
	buffer := make([]SampleItem, 0)

	store := s.storage.(storage.IteratePuller)
	reserveSizeOracle := s.storage.(depthmonitor.ReserveReporter)

	for bin := storageDepth; bin <= swarm.MaxPO; bin++ {
		chs, _, stop := store.IteratePull(ctx, bin, 0, 0)

		for ch := range chs {
			iterated++

			added = false

			chunk, err := s.storage.Get(ctx, storage.ModeGetSync, ch.Address)
			if err != nil {
				errored++
				s.logger.Error(err, "reserve sampler: skipping missing chunk")
				continue
			}

			hmacr.Write(chunk.Data())
			taddr := hmacr.Sum(nil)
			hmacr.Reset()

			for i, item := range buffer {
				distance, err := swarm.DistanceCmp(zerobytes, taddr, item.TransformedChunkAddress.Bytes())
				if err != nil {
					added = true
					errored++
					break
				}
				if distance > 0 {
					buffer = append(buffer[:i+1], buffer[i:]...)
					buffer[i] = SampleItem{ChunkAddress: ch.Address, TransformedChunkAddress: swarm.NewAddress(taddr)}
					added = true
					break
				}
				if distance == 0 {
					doubled++
					added = true
					break
				}
			}

			if len(buffer) > 16 {
				buffer = buffer[:16]
			}

			if len(buffer) < 16 && !added {
				buffer = append(buffer, SampleItem{ChunkAddress: ch.Address, TransformedChunkAddress: swarm.NewAddress(taddr)})
			}

		}

		stop()

	}

	reserveSize, _ := reserveSizeOracle.ReserveSize()

	if len(buffer) < 16 {
		return ReserveCommitment{}, iterated, doubled, errored, reserveSize, errors.New("not enough items")
	}
	hasher := swarm.NewHasher()

	for _, item := range buffer {
		hasher.Write(item.TransformedChunkAddress.Bytes())
	}
	hash := hasher.Sum(nil)

	return ReserveCommitment{Hash: swarm.NewAddress(hash), Items: buffer}, iterated, doubled, errored, reserveSize, nil

}

func (s *Sampler) Close() error {
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
