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
	_ = h.writeToLevel(1, p.span, p.ref)
	return 0, nil
}

func (h *hashTrieWriter) writeToLevel(level int, span, ref []byte) error {
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(span)], span) //copy the span slongside
	h.cursors[level] += len(span)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(ref)], ref)
	h.cursors[level] += len(ref)

	fmt.Println("write to level", level, "data", hex.EncodeToString(ref))
	if h.levelSize(level) == swarm.ChunkSize {
		h.wrapLevel(level)
	}

	return nil
}

func (h *hashTrieWriter) wrapLevel(level int) {
	fmt.Println("wrap level", level)
	/*
		wrapLevel does the following steps:
		 - take all of the data in the current level - OK
		 - break down span and hash data - OK
		 - sum the span size - OK
		 - call the short pipeline (that hashes and stores the intermediate chunk created)
		 - get the hash that was created, append it one level above, and if necessary, wrap that level too!
		 - remove already hashed data from buffer
	*/
	data := h.buffer[h.cursors[level+1]:h.cursors[level]]
	fmt.Println("level size", h.levelSize(level))
	sp := uint64(0)
	var hashes []byte
	for i := 0; i < len(data); i += h.refSize + 8 {
		// sum up the spans of the level, then we need to bmt them and store it as a chunk
		// then write the chunk address to the next level up
		sp += binary.LittleEndian.Uint64(data[i : i+8])
		fmt.Println("span on wrap", sp)
		hash := data[i+8 : i+h.refSize+8]
		fmt.Println("hash", hex.EncodeToString(hash))
		hashes = append(hashes, hash...)
	}
	spb := make([]byte, 8)
	binary.LittleEndian.PutUint64(spb, sp)
	hashes = append(spb, hashes...)
	fmt.Println("htw hashing level data", hex.EncodeToString(hashes))
	var results pipeWriteArgs
	writer := h.pipelineFn(&results)
	args := pipeWriteArgs{
		data: hashes,
	}
	writer.ChainWrite(&args)
	fmt.Println("got result on wrapping level", hex.EncodeToString(results.ref))
	h.writeToLevel(level+1, results.span, results.ref)
}

func (h *hashTrieWriter) levelSize(level int) int {
	return h.cursors[level] - h.cursors[level+1]
}

func (h *hashTrieWriter) Sum() ([]byte, error) {
	// sweep through the levels that have cursors .> 0
	for i := 1; i < 8; i++ {

		// theoretically, in an existing level, can be only upto 1 chunk of data, since
		// wrapping should occur on writes
		lSize := h.levelSize(i)

		// stop condition is: level size == size of reference + span (because we keep the spans in the buffer too) and no more data in the next level
		if lSize == h.refSize+8 && h.levelSize(i+1) == 0 {
			// return this ref
			return h.buffer[h.cursors[i+1]+8 : h.cursors[i]], nil
		}
		fmt.Println("level", i, "size", lSize)

		// if we have level size > 0, then it means only upto one chunk of data is there
		if lSize == 0 {
			fmt.Println(h.buffer[:1024])
			return nil, nil
		}
		h.wrapLevel(i)
	}

	return nil, nil
}
