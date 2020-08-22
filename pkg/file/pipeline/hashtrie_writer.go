package pipeline

import (
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/swarm"
)

type hashTrieWriter struct {
	branching int
	chunkSize int
	refLen    int

	length  int64  // how many bytes were written so far to the data layer
	cursors []int  // level cursors, key is level. level 0 is data level
	buffer  []byte // keeps all level data
}

func NewHashTrieWriter(chunkSize, branching, refLen int) EndPipeWriter {
	return &hashTrieWriter{
		brancing:  branching,
		chunkSize: chunkSize,
		refLen:    refLen,
	}
}

func (h *hashTrieWriter) SetHead(w ChainableWriter) {
	h.head = w
}

// accepts writes of hashes from the previous writer in the chain, by definition these writes
// are on level 1
func (h *hashTrieWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	_ = h.writeToLevel(1, p)
}

func (h *hashTrieWriter) writeToLevel(level int, p *pipeWriteArgs) error {
	copy(s.buffer[s.cursors[level]:s.cursors[level]+len(p.span)], p.span) //copy the span slongside
	s.cursors[level] += len(p.span)
	copy(s.buffer[s.cursors[lvl]:s.cursors[lvl]+len(data)], p.ref)
	s.cursors[lvl] += len(p.ref)

	if s.cursors[lvl]-s.cursors[lvl+1] == swarm.ChunkSize {
		h.wrapLevel(level)
	}
}

func (h *hashTrieWriter) wrapLevel(level int) {
	data := h.buffer[s.cursors[level+1]:s.cursors[level]]
	sp := 0
	for i := 0; i < len(data); i += h.refSize + 8 {
		// sum up the spans of the level, then we need to bmt them and store it as a chunk
		// then write the chunk address to the next level up
		sp += binary.LittleEndian.Uint64(data[i : i+8])
	}
}
