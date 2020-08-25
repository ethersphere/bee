// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import "errors"

type resultWriter struct {
	target *pipeWriteArgs
}

func NewResultWriter(b *pipeWriteArgs) ChainWriter {
	return &resultWriter{target: b}
}

func (w *resultWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	*w.target = *p
	return 0, nil
}

func (w *resultWriter) Sum() ([]byte, error) {
	return nil, errors.New("not implemented")
}
