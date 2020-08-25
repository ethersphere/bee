// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"context"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type storeWriter struct {
	l    storage.Putter
	next chainWriter
}

func NewStoreWriter(l storage.Putter, next chainWriter) chainWriter {
	return &storeWriter{l: l, next: next}
}

func (w *storeWriter) chainWrite(p *pipeWriteArgs) error {
	c := swarm.NewChunk(swarm.NewAddress(p.ref), p.data)
	_, err := w.l.Put(context.Background(), storage.ModePutUpload, c)
	if err != nil {
		return err
	}
	return w.next.chainWrite(p)
}

func (w *storeWriter) sum() ([]byte, error) {
	return w.next.sum()
}
