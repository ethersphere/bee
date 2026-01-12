// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
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

	if err := s.pin(r.Context(), paths.Reference); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			logger.Debug("pin root hash: pin failed", "chunk_address", paths.Reference, "error", err)
			logger.Error(nil, "pin root hash: pin failed")
			jsonhttp.NotFound(w, "pin collection failed")
			return
		}
		logger.Debug("pin root hash: pin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "pin root hash: pin failed")
		jsonhttp.InternalServerError(w, "pin root hash: pin failed")
		return
	}

	jsonhttp.Created(w, nil)
}

func (s *Service) pin(ctx context.Context, reference swarm.Address) error {
	has, err := s.storer.HasPin(reference)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	putter, err := s.storer.NewCollection(ctx)
	if err != nil {
		return err
	}

	getter := s.storer.Download(true)
	traverser := traversal.New(getter, s.storer.Cache(), redundancy.DefaultLevel)

	sem := semaphore.NewWeighted(100)
	var errTraverse error
	var mtxErr sync.Mutex
	var wg sync.WaitGroup

	err = traverser.Traverse(
		ctx,
		reference,
		func(address swarm.Address) error {
			mtxErr.Lock()
			if errTraverse != nil {
				mtxErr.Unlock()
				return errTraverse
			}
			mtxErr.Unlock()
			if err := sem.Acquire(ctx, 1); err != nil {
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
				chunk, err := getter.Get(ctx, address)
				if err != nil {
					return
				}
				err = putter.Put(ctx, chunk)
			}()
			return nil
		},
	)

	wg.Wait()

	if err := errors.Join(err, errTraverse); err != nil {
		return errors.Join(err, putter.Cleanup())
	}

	return putter.Done(reference)
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

	if err := s.unpin(r.Context(), paths.Reference); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			logger.Debug("unpin root hash: unpin failed", "chunk_address", paths.Reference, "error", err)
			logger.Error(nil, "unpin root hash: unpin failed")
			jsonhttp.NotFound(w, nil)
			return
		}
		logger.Debug("unpin root hash: unpin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "unpin root hash: unpin failed")
		jsonhttp.InternalServerError(w, "unpin root hash: unpin failed")
		return
	}

	jsonhttp.OK(w, nil)
}

func (s *Service) unpin(ctx context.Context, reference swarm.Address) error {
	has, err := s.storer.HasPin(reference)
	if err != nil {
		return err
	}
	if !has {
		return storage.ErrNotFound
	}

	return s.storer.DeletePin(ctx, reference)
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

	res, err := s.getPin(paths.Reference)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			jsonhttp.NotFound(w, nil)
			return
		}
		logger.Debug("pinned root hash: has pin failed", "chunk_address", paths.Reference, "error", err)
		logger.Error(nil, "pinned root hash: has pin failed")
		jsonhttp.InternalServerError(w, "pinned root hash: check reference failed")
		return
	}

	jsonhttp.OK(w, res)
}

func (s *Service) getPin(ref swarm.Address) (*struct {
	Reference swarm.Address `json:"reference"`
}, error) {
	has, err := s.storer.HasPin(ref)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, storage.ErrNotFound
	}

	return &struct {
		Reference swarm.Address `json:"reference"`
	}{
		Reference: ref,
	}, nil
}

// listPinnedRootHashes lists all the references of the pinned root hashes.
func (s *Service) listPinnedRootHashes(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pins").Build()

	pinned, err := s.listPins()
	if err != nil {
		logger.Debug("list pinned root references: unable to list references", "error", err)
		logger.Error(nil, "list pinned root references: unable to list references")
		jsonhttp.InternalServerError(w, "list pinned root references failed")
		return
	}

	jsonhttp.OK(w, pinned)
}

func (s *Service) listPins() (*struct {
	References []swarm.Address `json:"references"`
}, error) {
	pinned, err := s.storer.Pins()
	if err != nil {
		return nil, err
	}

	return &struct {
		References []swarm.Address `json:"references"`
	}{
		References: pinned,
	}, nil
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
