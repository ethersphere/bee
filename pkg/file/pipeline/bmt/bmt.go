// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"errors"
	"hash"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

var (
	errInvalidData = errors.New("bmt: invalid data")
)

type bmtWriter struct {
	b    bmt.Hash
	next pipeline.ChainWriter
}

// NewBmtWriter returns a new bmtWriter. Partial writes are not supported.
// Note: branching factor is the BMT branching factor, not the merkle trie branching factor.
func NewBmtWriter(branches int, next pipeline.ChainWriter) pipeline.ChainWriter {
	return &bmtWriter{
		b:    bmtlegacy.New(bmtlegacy.NewTreePool(hashFunc, branches, bmtlegacy.PoolSize)),
		next: next,
	}
}

// ChainWrite writes data in chain. It assumes span has been prepended to the data.
// The span can be encrypted or unencrypted.
func (w *bmtWriter) ChainWrite(p *pipeline.PipeWriteArgs) error {
	if len(p.Data) < swarm.SpanSize {
		return errInvalidData
	}
	w.b.Reset()
	err := w.b.SetSpanBytes(p.Data[:swarm.SpanSize])
	if err != nil {
		return err
	}
	_, err = w.b.Write(p.Data[swarm.SpanSize:])
	if err != nil {
		return err
	}
	p.Ref = w.b.Sum(nil)
	return w.next.ChainWrite(p)
}

// sum calls the next writer for the cryptographic sum
func (w *bmtWriter) Sum() ([]byte, error) {
	return w.next.Sum()
}

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}
