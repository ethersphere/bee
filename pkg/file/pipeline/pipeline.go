package pipeline

import (
	"io"

	"github.com/ethersphere/bee/pkg/storage"
)

type pipeWriteArgs struct {
	ref  []byte
	key  []byte
	span []byte
	data []byte //data includes the span too!
}

type Pipeline struct {
	head io.Writer
	tail EndPipeWriter
}

func (p *Pipeline) Write(b []byte) (int, error) {
	return p.head.Write(b)
}

func (p *Pipeline) Sum() ([]byte, error) {
	return p.tail.Sum()
}

func NewPipeline(s storage.Storer) Interface {
	tw := NewHashTrieWriter(4096, 128, 32, NewShortPipeline(s))
	lsw := NewStoreWriter(s, tw)
	b := NewBmtWriter(128, lsw)

	return &Pipeline{head: b, tail: tw}
}

type pipelineFunc func(p *pipeWriteArgs) io.Writer

// this is just a hashing pipeline. needed for level wrapping inside the hash trie writer
func NewShortPipeline(s storage.Storer) func(*pipeWriteArgs) io.Writer {
	return func(p *pipeWriteArgs) io.Writer {
		rsw := NewResultWriter(p)
		lsw := NewStoreWriter(s, rsw)
		bw := NewBmtWriter(128, lsw)

		return bw
	}
}

//func NewEncryptionPipeline() EndPipeWriter {
//tw := NewHashTrieWriter()
//lsw := NewStoreWriter()
//b := NewEncryptingBmtWriter(128, lsw) // needs to pass the key somehwoe...
//enc := NewEncryptionWriter(b)
//return
//}
