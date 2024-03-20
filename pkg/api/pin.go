// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/storage"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/traversal"
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
	traverser := traversal.New(getter, s.storer.Cache())

	sem := semaphore.NewWeighted(100)
	var errTraverse error
	var mtxErr sync.Mutex
	var wg sync.WaitGroup

	err = traverser.Traverse(
		r.Context(),
		paths.Reference,
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

	jsonhttp.OK(w, struct {
		Reference swarm.Address `json:"reference"`
	}{
		Reference: paths.Reference,
	})
}

// listPinnedRootHashes lists all the references of the pinned root hashes.
func (s *Service) listPinnedRootHashes(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pins").Build()

	pinned, err := s.storer.Pins()
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

type PinIntegrityResponse struct {
	Reference swarm.Address `json:"reference"`
	Total     int           `json:"total"`
	Missing   int           `json:"missing"`
	Invalid   int           `json:"invalid"`
}

func (s *Service) pinIntegrityHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pin_integrity").Build()

	querie := struct {
		Ref swarm.Address `map:"ref"`
	}{}

	if response := s.mapStructure(r.URL.Query(), &querie); response != nil {
		response("invalid query params", logger, w)
		return
	}

	out := make(chan storer.PinStat)

	go s.pinIntegrity.Check(r.Context(), logger, querie.Ref.String(), out)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	for v := range out {
		resp := PinIntegrityResponse{
			Reference: v.Ref,
			Total:     v.Total,
			Missing:   v.Missing,
			Invalid:   v.Invalid,
		}
		if err := enc.Encode(resp); err != nil {
			break
		}
		flusher.Flush()
	}
}
