// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stamp

import (
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type stampWriter struct {
	stamper postage.Stamper
	next    pipeline.ChainWriter
}

func NewStampWriter(stamper postage.Stamper, next pipeline.ChainWriter) pipeline.ChainWriter {
	return &stampWriter{stamper: stamper, next: next}
}

func (w *stampWriter) ChainWrite(p *pipeline.PipeWriteArgs) (err error) {
	addr := swarm.NewAddress(p.Ref)
	p.Stamp, err = w.stamper.Stamp(addr)
	if err != nil {
		return err
	}

	return w.next.ChainWrite(p)
}

func (w *stampWriter) Sum() ([]byte, error) {
	return w.next.Sum()
}
