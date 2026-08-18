// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errInvalidData = errors.New("store: invalid data")

type storeWriter struct {
	l    storage.Putter
	ctx  context.Context
	next pipeline.ChainWriter
}

// NewStoreWriter returns a storeWriter. It just writes the given data
// to a given storage.Putter.
func NewStoreWriter(ctx context.Context, l storage.Putter, next pipeline.ChainWriter) pipeline.ChainWriter {
	return &storeWriter{ctx: ctx, l: l, next: next}
}

func (w *storeWriter) ChainWrite(p *pipeline.PipeWriteArgs) error {
	if p.Ref == nil || p.Data == nil {
		return errInvalidData
	}
	ch := swarm.NewChunk(swarm.NewAddress(p.Ref), p.Data)
	err := w.l.Put(w.ctx, ch)
	if err != nil {
		return err
	}
	// the putter may have attached a postage stamp to the chunk (the api
	// putter mutates the chunk object in place); surface it to the
	// downstream writers so the hashtrie can collect it. storeWriter has no
	// logger and none is reachable through ctx without changing
	// NewStoreWriter's signature, so a marshal failure here is not logged: it
	// silently omits this chunk's stamp from the parent's carriers (the chunk
	// itself is still stored and the upload still succeeds). This is a
	// deliberate degradation, not a bug - carriers are best-effort recovery
	// metadata, never required for the chunk to be retrievable.
	if st := ch.Stamp(); st != nil {
		if b, err := st.MarshalBinary(); err == nil {
			p.Stamp = b
		}
	}
	if w.next == nil {
		return nil
	}

	return w.next.ChainWrite(p)
}

func (w *storeWriter) Sum() ([]byte, error) {
	return w.next.Sum()
}
