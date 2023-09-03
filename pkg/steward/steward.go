// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stewardess provides convenience methods
// for reseeding content on Swarm.
package steward

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/storage"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/traversal"
)

type Interface interface {
	// Reupload root hash and all of its underlying
	// associated chunks to the network.
	Reupload(context.Context, swarm.Address, postage.Stamper) error

	// IsRetrievable checks whether the content
	// on the given address is retrievable.
	IsRetrievable(context.Context, swarm.Address) (bool, error)

	Track(context.Context, swarm.Address) (bool, []*ChunkInfo, error)
}

type ChunkInfo struct {
	Address swarm.Address       `json:"address"`
	Batch string                `json:"batch"`
//	PinCount uint64             `json:"pinCount"`
}

type steward struct {
	netStore     storer.NetStore
	traverser    traversal.Traverser
	netTraverser traversal.Traverser
}

func New(ns storer.NetStore, r retrieval.Interface) Interface {
	return &steward{
		netStore:     ns,
		traverser:    traversal.New(ns.Download(true)),
		netTraverser: traversal.New(&netGetter{r}),
	}
}

// Reupload content with the given root hash to the network.
// The service will automatically dereference and traverse all
// addresses and push every chunk individually to the network.
// It assumes all chunks are available locally. It is therefore
// advisable to pin the content locally before trying to reupload it.
func (s *steward) Reupload(ctx context.Context, root swarm.Address, stamper postage.Stamper) error {
	uploaderSession := s.netStore.DirectUpload()
	getter := s.netStore.Download(false)

	fn := func(addr swarm.Address) error {
		c, err := getter.Get(ctx, addr)
		if err != nil {
			return err
		}

		stamp, err := stamper.Stamp(c.Address())
		if err != nil {
			return fmt.Errorf("stamping chunk %s: %w", c.Address(), err)
		}

		return uploaderSession.Put(ctx, c.WithStamp(stamp))
	}

	if err := s.traverser.Traverse(ctx, root, false, fn); err != nil {
		return errors.Join(
			fmt.Errorf("traversal of %s failed: %w", root.String(), err),
			uploaderSession.Cleanup(),
		)
	}

	return uploaderSession.Done(root)
}

// IsRetrievable implements Interface.IsRetrievable method.
func (s *steward) IsRetrievable(ctx context.Context, root swarm.Address) (bool, error) {
	noop := func(leaf swarm.Address) error { return nil }
	switch err := s.netTraverser.Traverse(ctx, root, false, noop); {
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	case errors.Is(err, topology.ErrNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("traversal of %q failed: %w", root, err)
	default:
		return true, nil
	}
}

// Track content with the given root hash to the network.
// The service will automatically dereference and traverse all
// addresses.
// It assumes all chunks are available. It is therefore
// advisable to pin the content locally before tracking it.
func (s *steward) Track(ctx context.Context, root swarm.Address) (bool, []*ChunkInfo, error) {

	var chunks []*ChunkInfo
	getter := s.netStore.Download(false)

	fn := func(addr swarm.Address) error {
		c, err := getter.Get(ctx, addr)
		if err != nil {
			//s.logger.Debug("steward:Track getter.Get failed", "chunk", addr, "err", err)
			return err
		}
		//stampBytes, err := c.Stamp().MarshalBinary()
		//if err != nil {
		//	return fmt.Errorf("pusher: valid stamp marshal: %w", err)
		//}
		batchID := "";
		if (c.Stamp() != nil) {
			batchID = hex.EncodeToString(c.Stamp().BatchID())
		}
		//checkFor := "0e8366a6fdac185b6f0327dc89af99e67d9d3b3f2af22432542dc5971065c1df"
		//if (batchID != checkFor) {
		//	s.logger.Debug("steward:Track", "chunk", addr, "batchID", batchID, "not", checkFor)
		//}
		//pc, err := s.getter.PinCounter(addr)
		//if err != nil {
		//	pc = 0
		//}
		//if pc == 0 {
		//	s.logger.Debug("steward:Track NOT pinned!", "chunk", addr, "batchID", batchID)
		//}
		chunks = append(
			chunks,
			&ChunkInfo{
				Address: addr,
				Batch: batchID,
		//		PinCount: pc,
			},
		)
		return nil
	}

	if err := s.traverser.Traverse(ctx, root, false, fn); err != nil {
		return false, chunks, fmt.Errorf("traversal of %s failed: %w", root.String(), err)
	}

	return true, chunks, nil
}

// netGetter implements the storage Getter.Get method in a way
// that it will try to retrieve the chunk only from the network.
type netGetter struct {
	retrieval retrieval.Interface
}

// Get implements the storage Getter.Get interface.
func (ng *netGetter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return ng.retrieval.RetrieveChunk(ctx, addr, swarm.ZeroAddress)
}
