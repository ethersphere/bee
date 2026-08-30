// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// executeHost serves the swarm host calls of a single execution. It is built
// per request in executeHandler and closed after the run, so the upload session
// it opens spans one execution and no more.
type executeHost struct {
	s        *Service
	logger   log.Logger
	cache    bool
	maxBytes uint64

	// sessionCtx scopes the upload session to the request rather than to the
	// run. A host call's own context is the watchdog's, and that is cancelled
	// the moment Execute returns — before Close commits — so a session opened
	// on it can only ever fail to commit. The request context is what every
	// other upload endpoint uses, and it is still live at Close.
	sessionCtx context.Context

	// mu guards the lazily opened session. A guest is single-threaded and
	// nested executions are sequential, but the session outlives individual
	// calls and is cheap to guard.
	mu      sync.Mutex
	batchID []byte
	session storer.PutterSession
}

var _ compute.Host = (*executeHost)(nil)

// newExecuteHost builds the per-request host. ctx is the request's, and outlives
// the run so the upload session can still be committed. maxBytes is the
// execution's byte budget, used to refuse an oversized download before it is
// materialised.
func (s *Service) newExecuteHost(ctx context.Context, logger log.Logger, maxBytes uint64) *executeHost {
	return &executeHost{s: s, logger: logger, cache: true, maxBytes: maxBytes, sessionCtx: ctx}
}

// BytesGet reassembles data of arbitrary length, as GET /bytes does.
func (h *executeHost) BytesGet(ctx context.Context, addr swarm.Address) ([]byte, error) {
	reader, l, err := joiner.New(ctx, h.s.storer.Download(h.cache), h.s.storer.Cache(), addr, redundancy.DefaultDownloadLevel)
	if err != nil {
		return nil, mapHostErr(err)
	}
	// The span is known up front, so an oversized object is refused without
	// reading it. readCapped still bounds the read for a lying or absent span.
	if h.maxBytes > 0 && l >= 0 && uint64(l) > h.maxBytes {
		return nil, compute.ErrTooLarge
	}
	data, err := readCapped(reader, h.maxBytes)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return nil, compute.ErrTooLarge
		}
		return nil, mapHostErr(err)
	}
	return data, nil
}

// BytesPut splits data of arbitrary length through the same pipeline POST
// /bytes uses and returns the root reference.
//
// Encryption and redundancy are deliberately not exposed to the guest: an
// encrypted reference is 64 bytes and the guest ABI writes a fixed 32.
func (h *executeHost) BytesPut(ctx context.Context, batchID, data []byte) (swarm.Address, error) {
	putter, err := h.putter(batchID)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	addr, err := requestPipelineFn(putter, false, redundancy.DefaultUploadLevel)(ctx, bytes.NewReader(data))
	if err != nil {
		return swarm.ZeroAddress, mapHostErr(err)
	}
	return addr, nil
}

// ChunkGet retrieves a single chunk, as GET /chunks/{addr} does.
func (h *executeHost) ChunkGet(ctx context.Context, addr swarm.Address) ([]byte, error) {
	chunk, err := h.s.storer.Download(h.cache).Get(ctx, addr)
	if err != nil {
		return nil, mapHostErr(err)
	}
	return chunk.Data(), nil
}

// ChunkPut stores a single content-addressed chunk verbatim. Unlike POST
// /chunks there is no single owner chunk path: a SOC needs a signature the
// guest has no way to produce.
func (h *executeHost) ChunkPut(ctx context.Context, batchID, data []byte) (swarm.Address, error) {
	putter, err := h.putter(batchID)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	chunk, err := cac.NewWithDataSpan(data)
	if err != nil {
		// Malformed chunk bytes are the guest's mistake, not the node's.
		h.logger.Debug("execute host: invalid chunk data", "error", err)
		return swarm.ZeroAddress, compute.ErrInvalid
	}
	if err := putter.Put(ctx, chunk); err != nil {
		return swarm.ZeroAddress, mapHostErr(err)
	}
	return chunk.Address(), nil
}

// putter returns the execution's upload session, opening it on the first put so
// a module that never uploads never creates one. It takes no context: the
// session is scoped to sessionCtx, not to the call that happened to open it.
//
// One execution gets one session and therefore one batch: a put with a
// different batch than the one that opened it is refused.
func (h *executeHost) putter(batchID []byte) (storer.PutterSession, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.session != nil {
		if !bytes.Equal(h.batchID, batchID) {
			h.logger.Debug("execute host: second batch refused")
			return nil, compute.ErrDenied
		}
		return h.session, nil
	}

	// The upload store rejects a zero tag, so a session id is always allocated.
	tag, err := h.s.getOrCreateSessionID(0)
	if err != nil {
		return nil, err
	}
	session, err := h.s.newStamperPutter(h.sessionCtx, putterOptions{
		BatchID: batchID,
		TagID:   tag,
		// Deferred: a put returns once the chunk is stored locally and the
		// pusher syncs it afterwards. A direct upload would block on network
		// round trips inside a host call and burn the watchdog.
		Deferred: true,
	})
	if err != nil {
		return nil, mapHostErr(err)
	}

	h.batchID = bytes.Clone(batchID)
	h.session = session
	return session, nil
}

// Close finalises the upload session, if one was opened. A committed session
// hands its chunks to the pusher; otherwise they are dropped, so a module that
// trapped leaves nothing behind.
func (h *executeHost) Close(commit bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.session == nil {
		return nil
	}
	session := h.session
	h.session = nil

	if commit {
		return session.Done(swarm.ZeroAddress)
	}
	return session.Cleanup()
}

// mapHostErr translates a node error into the sentinel the guest may observe.
// Anything unrecognised is returned as-is and ends the execution as
// StatusHostError: a node-local failure is never a program verdict.
func mapHostErr(err error) error {
	switch {
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, topology.ErrNotFound):
		return errors.Join(compute.ErrNotFound, err)
	case errors.Is(err, errBatchUnusable),
		errors.Is(err, errInvalidPostageBatch),
		errors.Is(err, postage.ErrNotFound),
		errors.Is(err, postage.ErrNotUsable),
		errors.Is(err, postage.ErrBucketFull),
		errors.Is(err, postage.ErrInvalidBatchSignature):
		return errors.Join(compute.ErrDenied, err)
	default:
		return err
	}
}
