package file

import (
	"context"
	"hash"
	"io"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

type Pipeline struct {
	head ChainableWriter
	tail EndPipeWriter
}

func NewPipeline() EndPipeWriter {
	tw := NewHashTrieWriter(4096, 128)
	lsw := NewStoreWriter(nil, tw)
	b := NewBmtWriter(128, lsw)
	return &Pipeline{head: b, tail: tw}
}

func (p *Pipeline) Write(b []byte) (int, error) {
	return p.head.Write(b)
}

func (p *Pipeline) Sum() ([]byte, error) {
	return p.tail.Sum()
}

//func NewEncryptionPipeline() EndPipeWriter {
//tw := NewHashTrieWriter()
//lsw := NewStoreWriter()
//b := NewEncryptingBmtWriter(128, lsw) // needs to pass the key somehwoe...
//enc := NewEncryptionWriter(b)
//return
//}

type ChainableWriter interface {
	io.Writer
	//io.Flusher // not sure if this is really needed
}

// this one is by definition not chainable and is used in the end of the pipeline
// in order to execute any pending operations
type EndPipeWriter interface {
	ChainableWriter
	Sum() ([]byte, error)
}

type bmtWriter struct {
	b    bmt.Hash
	next ChainableWriter
}

// branches is the branching factor for BMT(!), not the same like in the trie of hashes which can differ between encrypted and unencrypted content
func NewBmtWriter(branches int, next ChainableWriter) ChainableWriter {
	return &bmtWriter{b: bmtlegacy.New(bmtlegacy.NewTreePool(hashFunc, branches, bmtlegacy.PoolSize))}
}

// Write assumes that the span is prepended to the actual data before the write !
func (w *bmtWriter) Write(b []byte) (int, error) {
	w.b.Reset()
	err := w.b.SetSpanBytes(b[:8])
	if err != nil {
		return nil, err
	}
	_, err = w.b.Write(b[8:])
	if err != nil {
		return nil, err
	}
	bytes := w.b.Sum(nil)
	return w.next.Write(bytes)
}

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}

type storeWriter struct {
	l    storage.Putter
	next ChainableWriter
}

func NewStoreWriter(l *storage.Putter, next ChainableWriter) ChainableWriter {
	return &storeWriter{l: l, next: next}
}

// Write assumes that the bmt hash is prepended to the actual data!
func (w *storeWriter) Write(b []byte) (int, error) {
	a := swarm.NewAddress(b[:32])
	c := swarm.NewChunk(a, b[32:])
	w.l.Put(context.Background(), c)
	return w.next.Write(a)
}

type hashTrieWriter struct {
	branching int
	chunkSize int

	length  int64  // how many bytes were written so far to the data layer
	cursors []int  // level cursors, key is level. level 0 is data level
	buffer  []byte // keeps all level data
}

func NewHashTrieWriter(chunkSize, branching int) EndPipeWriter {
	return &hashTrieWriter{
		brancing:  branching,
		chunkSize: chunkSize,
	}
}

// accepts writes of hashes from the previous writer in the chain, by definition these writes
// are on level 1
func (h *hashTrieWriter) Write(b []byte) (int, error) {
	_ = h.writeToLevel(1, b)
}

func (h *hashTrieWriter) writeToLevel(level int, b []byte) error {
	copy(s.buffer[s.cursors[lvl]:s.cursors[lvl]+len(data)], data)
	s.cursors[lvl] += len(data)
	if s.cursors[lvl] == swarm.ChunkSize {
		h.wrapLevel(level)
	}
}

func (h *hashTrieWriter) wrapLevel(level int) {
	data := h.buffer[s.cursors[level+1]:s.cursors[level]]
}
