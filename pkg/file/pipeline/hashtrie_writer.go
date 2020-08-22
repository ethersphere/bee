package pipeline

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/ethersphere/bee/pkg/swarm"
)

type hashTrieWriter struct {
	branching int
	chunkSize int
	refSize   int

	length     int64  // how many bytes were written so far to the data layer
	cursors    []int  // level cursors, key is level. level 0 is data level
	buffer     []byte // keeps all level data
	pipelineFn pipelineFunc
}

func NewHashTrieWriter(chunkSize, branching, refLen int, pipelineFn pipelineFunc) EndPipeWriter {
	return &hashTrieWriter{
		cursors:    make([]int, 9),
		buffer:     make([]byte, swarm.ChunkWithSpanSize*9*2), // double size as temp workaround for weak calculation of needed buffer space
		branching:  branching,
		chunkSize:  chunkSize,
		refSize:    refLen,
		pipelineFn: pipelineFn,
	}
}

// accepts writes of hashes from the previous writer in the chain, by definition these writes
// are on level 1
func (h *hashTrieWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	_ = h.writeToLevel(1, p)
	return 0, nil
}

func (h *hashTrieWriter) writeToLevel(level int, p *pipeWriteArgs) error {
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(p.span)], p.span) //copy the span slongside
	h.cursors[level] += len(p.span)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(p.ref)], p.ref)
	h.cursors[level] += len(p.ref)

	fmt.Println("write to level", level, "data", hex.EncodeToString(p.ref))
	if h.cursors[level]-h.cursors[level+1] == swarm.ChunkSize {
		h.wrapLevel(level)
	}

	return nil
}

func (h *hashTrieWriter) wrapLevel(level int) {
	/*
		wrapLevel does the following steps:
		 - take all of the data in the current level
		 - break down span and hash data
		 - sum the span size
		 - call the short pipeline (that hashes and stores the intermediate chunk created)
		 - get the hash that was created, append it one level above
		 - remove already hashed data from buffer
	*/
	data := h.buffer[h.cursors[level+1]:h.cursors[level]]
	sp := uint64(0)
	var hashes []byte
	for i := 0; i < len(data); i += h.refSize + 8 {
		// sum up the spans of the level, then we need to bmt them and store it as a chunk
		// then write the chunk address to the next level up
		sp += binary.LittleEndian.Uint64(data[i : i+8])
		hashes = append(hashes, data[i+8:i+h.refSize+8]...)
	}

	var results pipeWriteArgs
	writer := h.pipelineFn(&results)
	writer.Write(hashes)
}

func (h *hashTrieWriter) Sum() ([]byte, error) {
	return nil, nil
}
