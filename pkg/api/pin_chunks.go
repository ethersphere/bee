// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

// pinChunk pin's the already created chunk given its address.
// it fails if the chunk is not present in the local store.
// It also increments a pin counter to keep track of how many pin requests
// are originating for this chunk.
func (s *server) pinChunk(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("pin chunk: parse chunk address: %v", err)
		s.logger.Error("pin chunk: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	err = s.storer.Set(r.Context(), storage.ModeSetPin, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			ch, err := s.storer.Get(r.Context(), storage.ModeGetRequest, addr)
			if err != nil {
				s.logger.Debugf("pin chunk: netstore get: %v", err)
				s.logger.Error("pin chunk: netstore")

				jsonhttp.NotFound(w, nil)
				return
			}

			_, err = s.storer.Put(r.Context(), storage.ModePutRequestPin, ch)
			if err != nil {
				s.logger.Debugf("pin chunk: storer put pin: %v", err)
				s.logger.Error("pin chunk: storer put pin")

				jsonhttp.InternalServerError(w, err)
				return
			}
		} else {
			s.logger.Debugf("pin chunk: pinning error: %v, addr %s", err, addr)
			s.logger.Error("pin chunk: cannot pin chunk")

			jsonhttp.InternalServerError(w, "cannot pin chunk")
			return
		}
	}

	jsonhttp.OK(w, nil)
}

// unpinChunk unpin's an already pinned chunk. If the chunk is not present or the
// if the pin counter is zero, it raises error.
func (s *server) unpinChunk(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("pin chunk: parse chunk address: %v", err)
		s.logger.Error("pin chunk: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.storer.Has(r.Context(), addr)
	if err != nil {
		s.logger.Debugf("pin chunk: localstore has: %v", err)
		s.logger.Error("pin chunk: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	_, err = s.storer.PinCounter(addr)
	if err != nil {
		s.logger.Debugf("pin chunk: not pinned: %v", err)
		s.logger.Error("pin chunk: pin counter")
		jsonhttp.BadRequest(w, "chunk is not yet pinned")
		return
	}

	err = s.storer.Set(r.Context(), storage.ModeSetUnpin, addr)
	if err != nil {
		s.logger.Debugf("pin chunk: unpinning error: %v, addr %s", err, addr)
		s.logger.Error("pin chunk: unpin")
		jsonhttp.InternalServerError(w, "cannot unpin chunk")
		return
	}
	jsonhttp.OK(w, nil)
}

type pinnedChunk struct {
	Address    swarm.Address `json:"address"`
	PinCounter uint64        `json:"pinCounter"`
}

type listPinnedChunksResponse struct {
	Chunks []pinnedChunk `json:"chunks"`
}

// listPinnedChunks lists all the chunk address and pin counters that are currently pinned.
func (s *server) listPinnedChunks(w http.ResponseWriter, r *http.Request) {
	var (
		err           error
		offset, limit = 0, 100 // default offset is 0, default limit 100
	)

	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil {
			s.logger.Debugf("list pins: parse offset: %v", err)
			s.logger.Errorf("list pins: bad offset")
			jsonhttp.BadRequest(w, "bad offset")
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil {
			s.logger.Debugf("list pins: parse limit: %v", err)
			s.logger.Errorf("list pins: bad limit")
			jsonhttp.BadRequest(w, "bad limit")
		}
	}

	pinnedChunks, err := s.storer.PinnedChunks(r.Context(), offset, limit)
	if err != nil {
		s.logger.Debugf("list pins: list pinned: %v", err)
		s.logger.Errorf("list pins: list pinned")
		jsonhttp.InternalServerError(w, err)
		return
	}

	chunks := make([]pinnedChunk, len(pinnedChunks))
	for i, c := range pinnedChunks {
		chunks[i] = pinnedChunk(*c)
	}

	jsonhttp.OK(w, listPinnedChunksResponse{
		Chunks: chunks,
	})
}

func (s *server) getPinnedChunk(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("pin counter: parse chunk ddress: %v", err)
		s.logger.Errorf("pin counter: parse address")
		jsonhttp.NotFound(w, nil)
		return
	}

	has, err := s.storer.Has(r.Context(), addr)
	if err != nil {
		s.logger.Debugf("pin counter: localstore has: %v", err)
		s.logger.Errorf("pin counter: store")
		jsonhttp.NotFound(w, nil)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	pinCounter, err := s.storer.PinCounter(addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			jsonhttp.NotFound(w, nil)
			return
		}
		s.logger.Debugf("pin counter: get pin counter: %v", err)
		s.logger.Errorf("pin counter: get pin counter")
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, pinnedChunk{
		Address:    addr,
		PinCounter: pinCounter,
	})
}

type updatePinCounter struct {
	PinCounter uint64 `json:"pinCounter"`
}

// updatePinnedChunkPinCounter allows changing the pin counter for the chunk.
func (s *server) updatePinnedChunkPinCounter(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("update pin counter: parse chunk ddress: %v", err)
		s.logger.Errorf("update pin counter: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.storer.Has(r.Context(), addr)
	if err != nil {
		s.logger.Debugf("update pin counter: localstore has: %v", err)
		s.logger.Errorf("update pin counter: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	pinCounter, err := s.storer.PinCounter(addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			jsonhttp.NotFound(w, nil)
			return
		}
		s.logger.Debugf("pin counter: get pin counter: %v", err)
		s.logger.Errorf("pin counter: get pin counter")
		jsonhttp.InternalServerError(w, err)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debugf("update pin counter: read request body error: %v", err)
		s.logger.Error("update pin counter: read request body error")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	newPinCount := updatePinCounter{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &newPinCount)
		if err != nil {
			s.logger.Debugf("update pin counter: unmarshal pin counter error: %v", err)
			s.logger.Errorf("update pin counter: unmarshal pin counter error")
			jsonhttp.InternalServerError(w, "error unmarshaling pin counter")
			return
		}
	}

	if newPinCount.PinCounter > math.MaxInt32 {
		s.logger.Errorf("update pin counter: invalid pin counter %d", newPinCount.PinCounter)
		jsonhttp.BadRequest(w, "invalid pin counter")
		return
	}

	diff := newPinCount.PinCounter - pinCounter

	err = s.updatePinCount(r.Context(), addr, int(diff))
	if err != nil {
		s.logger.Debugf("update pin counter: update error: %v, addr %s", err, addr)
		s.logger.Error("update pin counter: update")
		jsonhttp.InternalServerError(w, err)
		return
	}

	pinCounter, err = s.storer.PinCounter(addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			pinCounter = 0
		} else {
			s.logger.Debugf("update pin counter: get pin counter: %v", err)
			s.logger.Errorf("update pin counter: get pin counter")
			jsonhttp.InternalServerError(w, err)
			return
		}
	}

	jsonhttp.OK(w, pinnedChunk{
		Address:    addr,
		PinCounter: pinCounter,
	})
}

// updatePinCount changes pin counter for a chunk address.
// This is done with a loop, depending on the delta value supplied.
// NOTE: If the value is too large, it will result in many database operations.
func (s *server) updatePinCount(ctx context.Context, reference swarm.Address, delta int) error {
	diff := delta
	mode := storage.ModeSetPin

	if diff < 0 {
		diff = -diff
		mode = storage.ModeSetUnpin
	}

	for i := 0; i < diff; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := s.storer.Set(ctx, mode, reference)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *server) pinChunkAddressFn(ctx context.Context, reference swarm.Address) func(address swarm.Address) error {
	return func(address swarm.Address) error {
		// NOTE: stop pinning on first error

		err := s.storer.Set(ctx, storage.ModeSetPin, address)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				// chunk not found locally, try to get from netstore
				ch, err := s.storer.Get(ctx, storage.ModeGetRequest, address)
				if err != nil {
					s.logger.Debugf("pin traversal: storer get: for reference %s, address %s: %w", reference, address, err)
					return err
				}

				_, err = s.storer.Put(ctx, storage.ModePutRequestPin, ch)
				if err != nil {
					s.logger.Debugf("pin traversal: storer put pin: for reference %s, address %s: %w", reference, address, err)
					return err
				}

				return nil
			}

			s.logger.Debugf("pin traversal: storer set pin: for reference %s, address %s: %w", reference, address, err)
			return err
		}

		return nil
	}
}

func (s *server) unpinChunkAddressFn(ctx context.Context, reference swarm.Address) func(address swarm.Address) error {
	return func(address swarm.Address) error {
		_, err := s.storer.PinCounter(address)
		if err != nil {
			return err
		}

		err = s.storer.Set(ctx, storage.ModeSetUnpin, address)
		if err != nil {
			s.logger.Debugf("unpin traversal: for reference %s, address %s: %w", reference, address, err)
			// continue un-pinning all chunks
		}

		return nil
	}
}
