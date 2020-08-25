// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"hash"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

type bmtWriter struct {
	b    bmt.Hash
	next chainWriter
}

// branches is the branching factor for BMT(!), not the same like in the trie of hashes which can differ between encrypted and unencrypted content
// NewBmtWriter returns a new bmtWriter. Partial writes are not supported.
// Note: branching factor is the BMT branching factor, not the merkle trie branching factor.
func NewBmtWriter(branches int, next chainWriter) chainWriter {
	return &bmtWriter{
		b:    bmtlegacy.New(bmtlegacy.NewTreePool(hashFunc, branches, bmtlegacy.PoolSize)),
		next: next,
	}
}

// chainWrite writes data in chain. It assumes span has been prepended to the data.
// The span can be encrypted or unencrypted.
func (w *bmtWriter) chainWrite(p *pipeWriteArgs) error {
	w.b.Reset()
	err := w.b.SetSpanBytes(p.data[:swarm.SpanSize])
	if err != nil {
		return err
	}
	_, err = w.b.Write(p.data[swarm.SpanSize:])
	if err != nil {
		return err
	}
	bytes := w.b.Sum(nil)
	args := &pipeWriteArgs{ref: bytes, data: p.data, span: p.data[:swarm.SpanSize]}
	return w.next.chainWrite(args)
}

// sum calls the next writer for the cryptographic sum
func (w *bmtWriter) sum() ([]byte, error) {
	return w.next.sum()
}

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}
