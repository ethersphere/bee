package pipeline

import (
	"hash"

	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

type bmtWriter struct {
	b    bmt.Hash
	next ChainWriter
}

// branches is the branching factor for BMT(!), not the same like in the trie of hashes which can differ between encrypted and unencrypted content
func NewBmtWriter(branches int, next ChainWriter) ChainWriter {
	return &bmtWriter{
		b:    bmtlegacy.New(bmtlegacy.NewTreePool(hashFunc, branches, bmtlegacy.PoolSize)),
		next: next,
	}
}

// Write assumes that the span is prepended to the actual data before the write !
func (w *bmtWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	w.b.Reset()
	err := w.b.SetSpanBytes(p.data[:8])
	if err != nil {
		return 0, err
	}
	_, err = w.b.Write(p.data[8:])
	if err != nil {
		return 0, err
	}
	bytes := w.b.Sum(nil)
	args := &pipeWriteArgs{ref: bytes, data: p.data, span: p.data[:8]}
	return w.next.ChainWrite(args)
}

func (w *bmtWriter) Sum() ([]byte, error) {
	return w.next.Sum()
}

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}
