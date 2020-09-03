// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"context"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

type storeWriter struct {
	l    storage.Putter
	mode storage.ModePut
	ctx  context.Context
	next chainWriter
}

// newStoreWriter returns a storeWriter. It just writes the given data
// to a given storage.Storer.
func newStoreWriter(ctx context.Context, l storage.Putter, mode storage.ModePut, next chainWriter) chainWriter {
	return &storeWriter{ctx: ctx, l: l, mode: mode, next: next}
}

func (w *storeWriter) chainWrite(p *pipeWriteArgs) error {
	tag := sctx.GetTag(w.ctx)
	var c swarm.Chunk
	if tag != nil {
		err := tag.Inc(tags.StateSplit)
		if err != nil {
			return err
		}
		c = swarm.NewChunk(swarm.NewAddress(p.ref), p.data).WithTagID(tag.Uid)
	} else {
		c = swarm.NewChunk(swarm.NewAddress(p.ref), p.data)
	}

	seen, err := w.l.Put(w.ctx, w.mode, c)
	if err != nil {
		return err
	}
	if tag != nil {
		err := tag.Inc(tags.StateStored)
		if err != nil {
			return err
		}
		if seen[0] {
			err := tag.Inc(tags.StateSeen)
			if err != nil {
				return err
			}
		}
	}
	if w.next == nil {
		return nil
	}

	return w.next.chainWrite(p)

}

func (w *storeWriter) sum() ([]byte, error) {
	return w.next.sum()
}
