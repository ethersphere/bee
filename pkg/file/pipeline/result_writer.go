// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import "errors"

type resultWriter struct {
	target *pipeWriteArgs
}

func NewResultWriter(b *pipeWriteArgs) chainWriter {
	return &resultWriter{target: b}
}

func (w *resultWriter) chainWrite(p *pipeWriteArgs) error {
	*w.target = *p
	return nil
}

func (w *resultWriter) sum() ([]byte, error) {
	return nil, errors.New("not implemented")
}
