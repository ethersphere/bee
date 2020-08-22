package pipeline

import (
	"fmt"
	"hash"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

type bmtWriter struct {
	b    bmt.Hash
	next ChainableWriter
}

// branches is the branching factor for BMT(!), not the same like in the trie of hashes which can differ between encrypted and unencrypted content
func NewBmtWriter(branches int, next ChainableWriter) io.Writer {
	return &bmtWriter{
		b:    bmtlegacy.New(bmtlegacy.NewTreePool(hashFunc, branches, bmtlegacy.PoolSize)),
		next: next,
	}
}

// PIPELINE IS: DATA -> BMT -> STORAGE -> TRIE

// Write assumes that the span is prepended to the actual data before the write !
func (w *bmtWriter) Write(b []byte) (int, error) {
	w.b.Reset()
	err := w.b.SetSpanBytes(b[:8])
	if err != nil {
		return 0, err
	}
	_, err = w.b.Write(b[8:])
	if err != nil {
		return 0, err
	}
	bytes := w.b.Sum(nil)
	fmt.Println("bmt hashed chunk", swarm.NewAddress(bytes).String())
	args := &pipeWriteArgs{ref: bytes, data: b, span: b[:8]}
	return w.next.ChainWrite(args)
}

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}
