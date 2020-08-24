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
	fullChunk int // full chunk size in terms of the data represented in the buffer (span+refsize)

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
		fullChunk:  (refLen + 8) * branching,
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

	//fmt.Println("write to level", level, "data", hex.EncodeToString(ref))
	howLong := (h.refSize + swarm.SpanSize) * h.branching
	if h.levelSize(level) == howLong {
		h.wrapFullLevel(level)
	}

	return nil
}

func (h *hashTrieWriter) wrapFullLevel(level int) {
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
		//fmt.Println("span on wrap", sp)
		hash := data[i+8 : i+h.refSize+8]
		hashes = append(hashes, hash...)
	}
	spb := make([]byte, 8)
	binary.LittleEndian.PutUint64(spb, sp)
	hashes = append(spb, hashes...)
	//fmt.Println("htw hashing level data", hex.EncodeToString(hashes))
	var results pipeWriteArgs
	writer := h.pipelineFn(&results)
	args := pipeWriteArgs{
		data: hashes,
	}
	writer.ChainWrite(&args)
	//fmt.Println("got result on wrapping level", hex.EncodeToString(results.ref))
	h.writeToLevel(level+1, results.span, results.ref)
	h.cursors[level] = h.cursors[level+1] // this "truncates" the current level that was wrapped
	// by setting the cursors the the cursors of one level above
}

// pulls and potentially wraps all levels up to target
func (h *hashTrieWriter) hoistLevels(target int) []byte {
	fmt.Println("hoist levels", target)
	for i := 1; i < target; i++ {
		l := h.levelSize(i)
		fmt.Println("i", i, "target", target, "levelsize", l)
		switch {
		case l == 0:
			continue
		case l == h.fullChunk:
			h.wrapFullLevel(i)
		case l > h.fullChunk:
			panic(1)
			for i := l; i > 0; {
			}
		default:
			fmt.Println(l)
			// more than 0 but smaller than chunk size:
			// just copy the data one level up

			// i think we can just get away with moving the cursors of the upper level to be those of this level
			h.cursors[i+1] = h.cursors[i] // this is not right if the level is target-1
		}
	}
	level := target
	tlen := h.levelSize(target)
	data := h.buffer[h.cursors[level+1]:h.cursors[level]]
	if tlen == h.refSize+8 {
		return data[8:]
	}

	fmt.Println("len target", len(h.buffer[h.cursors[target+1]:h.cursors[target]]))

	// here we are still with possible length of more than one ref in the highest+1 level
	fmt.Println("level size", h.levelSize(level))
	sp := uint64(0)
	var hashes []byte
	for i := 0; i < len(data); i += h.refSize + 8 {
		// sum up the spans of the level, then we need to bmt them and store it as a chunk
		// then write the chunk address to the next level up
		sp += binary.LittleEndian.Uint64(data[i : i+8])
		//fmt.Println("span on wrap", sp)
		hash := data[i+8 : i+h.refSize+8]
		hashes = append(hashes, hash...)
	}
	spb := make([]byte, 8)
	binary.LittleEndian.PutUint64(spb, sp)
	hashes = append(spb, hashes...)
	//fmt.Println("htw hashing level data", hex.EncodeToString(hashes))
	var results pipeWriteArgs
	writer := h.pipelineFn(&results)
	args := pipeWriteArgs{
		data: hashes,
	}
	writer.ChainWrite(&args)
	fmt.Println(hex.EncodeToString(h.buffer)[:1024])

	return results.ref
}

func (h *hashTrieWriter) levelSize(level int) int {
	if level == 8 {
		return h.cursors[level]
	}
	return h.cursors[level] - h.cursors[level+1]
}

func (h *hashTrieWriter) Sum() ([]byte, error) {
	// look from the top down, to look for the highest hash of a balanced tree
	// then, whatever is in the levels below that is necessarily unbalanced,
	// so, we'd like to reduce those levels to one hash, then wrap it together
	// with the balanced tree hash, to produce the root chunk
	highest := 1
	for i := 8; i > 0; i-- {
		if h.levelSize(i) > 0 && i > highest {
			highest = i
		}
	}

	ref := h.hoistLevels(highest)

	fmt.Println("ref", hex.EncodeToString(ref))
	return ref, nil
}
