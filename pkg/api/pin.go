// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/gorilla/mux"
	"golang.org/x/sync/semaphore"
)

// pinRootHash pins root hash of given reference. This method is idempotent.
func (s *Service) pinRootHash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_pin").Build()

	paths := struct {
		Reference swarm.Address `map:"reference" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	has, err := s.storer.HasPin(paths.Reference)
	if err != nil {
		logger.Debug("pin root hash: has pin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "pin root hash: has pin failed")
		jsonhttp.InternalServerError(w, "pin root hash: checking of tracking pin failed")
		return
	}
	if has {
		jsonhttp.OK(w, nil)
		return
	}

	putter, err := s.storer.NewCollection(r.Context())
	if err != nil {
		logger.Debug("pin root hash: failed to create collection", "error", err)
		logger.Error(nil, "pin root hash: failed to create collection")
		jsonhttp.InternalServerError(w, "pin root hash: create collection failed")
		return
	}

	getter := s.storer.Download(true)
	traverser := traversal.New(getter)

	sem := semaphore.NewWeighted(100)
	var errTraverse error
	var mtxErr sync.Mutex
	var wg sync.WaitGroup

	err = traverser.Traverse(
		r.Context(),
		paths.Reference,
		false,	// Do not iterate through manifests
		func(address swarm.Address) error {
			mtxErr.Lock()
			if errTraverse != nil {
				mtxErr.Unlock()
				return errTraverse
			}
			mtxErr.Unlock()
			if err := sem.Acquire(r.Context(), 1); err != nil {
				return err
			}
			wg.Add(1)
			go func() {
				var err error
				defer func() {
					sem.Release(1)
					wg.Done()
					if err != nil {
						mtxErr.Lock()
						errTraverse = errors.Join(errTraverse, err)
						mtxErr.Unlock()
					}
				}()
				chunk, err := getter.Get(r.Context(), address)
				if err != nil {
					return
				}
				err = putter.Put(r.Context(), chunk)
			}()
			return nil
		},
	)

	wg.Wait()

	if err := errors.Join(err, errTraverse); err != nil {
		logger.Error(errors.Join(err, putter.Cleanup()), "pin collection failed")
		if errors.Is(err, storage.ErrNotFound) {
			jsonhttp.NotFound(w, "pin collection failed")
			return
		}
		jsonhttp.InternalServerError(w, "pin collection failed")
		return
	}

	err = putter.Done(paths.Reference)
	if err != nil {
		logger.Debug("pin collection failed on done", "error", err)
		logger.Error(nil, "pin collection failed")
		jsonhttp.InternalServerError(w, "pin collection failed")
		return
	}

	jsonhttp.Created(w, nil)
}

// unpinRootHash unpin's an already pinned root hash. This method is idempotent.
func (s *Service) unpinRootHash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("delete_pin").Build()

	paths := struct {
		Reference swarm.Address `map:"reference" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	has, err := s.storer.HasPin(paths.Reference)
	if err != nil {
		logger.Debug("unpin root hash: has pin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "unpin root hash: has pin failed")
		jsonhttp.InternalServerError(w, "pin root hash: checking of tracking pin")
		return
	}
	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	if err := s.storer.DeletePin(r.Context(), paths.Reference); err != nil {
		logger.Debug("unpin root hash: delete pin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "unpin root hash: delete pin failed")
		jsonhttp.InternalServerError(w, "unpin root hash: deletion of pin failed")
		return
	}

	jsonhttp.OK(w, nil)
}


type refInfo struct {
	Address swarm.Address       `json:"address"`
	Error   bool                `json:"err"`
	Local   bool                `json:"local"`
	Cached  bool                `json:"cached"`
	RefCnt  uint32              `json:"refCnt"`
}

type getResponse struct {
	Reference swarm.Address `json:"reference"`
	ChunkCount int64 `json:"chunkCount"`
	Chunks []*refInfo `json:"chunks"`
}

// getPinnedRootHash returns back the given reference if its root hash is pinned.
func (s *Service) getPinnedRootHash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pin").Build()

	paths := struct {
		Reference swarm.Address `map:"reference" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	has, err := s.storer.HasPin(paths.Reference)
	if err != nil {
		logger.Debug("pinned root hash: has pin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "pinned root hash: has pin failed")
		jsonhttp.InternalServerError(w, "pinned root hash: check reference failed")
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	var chunks []*swarm.Address
	var nChunks int64
	err = s.storer.IteratePinCollection(paths.Reference, func(addr swarm.Address) (stop bool, err error) {
		chunks = append(chunks, &addr)
		nChunks++
		return false, nil
	})

	if err != nil {
		logger.Debug("pinned root hash: iterate pin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "pinned root hash: iterate pin failed")
		jsonhttp.InternalServerError(w, "pinned root hash: iterate reference failed")
		return
	}
	
	var refs []*refInfo
	for _, c := range chunks {
		good := true
		h, e := s.storer.ChunkStore().Has(context.Background(), *c)
		if e != nil {
			good = false
			logger.Debug("pinned root hash: ChunkStore.Has failed", "chunk_address", *c, "error", e)
		}
		r, e := s.storer.ChunkStore().GetRefCnt(context.Background(), *c)
		if e != nil {
			good = false
			logger.Debug("pinned root hash: ChunkStore.GetRefCnt failed", "chunk_address", *c, "error", e)
		}
		cached, e := s.storer.IsCached(*c)
		if e != nil {
			good = false
			logger.Debug("pinned root hash: CacheStore.IsCached failed", "chunk_address", *c, "error", e)
		}
		refs = append(refs, &refInfo{
			Address: *c,
			Error: !good,
			Local: h,
			RefCnt: r,
			Cached: cached,
		})
		
	}

	jsonhttp.OK(w, getResponse{
		Reference: paths.Reference,
		ChunkCount: nChunks,
		Chunks: refs,
	})

//	jsonhttp.OK(w, struct {
//		Reference swarm.Address `json:"reference"`
//	}{
//		Reference: paths.Reference,
//	})
}

// listPinnedRootHashes lists all the references of the pinned root hashes.
func (s *Service) listPinnedRootHashes(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pins").Build()

	queries := struct {
		Offset    int  `map:"offset"`
		Limit     int  `map:"limit"`
	}{}

	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	pinned, err := s.storer.Pins(queries.Offset, queries.Limit)
	if err != nil {
		logger.Debug("list pinned root references: unable to list references", "error", err)
		logger.Error(nil, "list pinned root references: unable to list references")
		jsonhttp.InternalServerError(w, "list pinned root references failed")
		return
	}

	jsonhttp.OK(w, struct {
		References []swarm.Address `json:"references"`
	}{
		References: pinned,
	})
}
