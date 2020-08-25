package pipeline

import (
	"encoding/binary"
)

const span = 8

type chunkFeeder struct {
	size      int
	next      ChainWriter
	buffer    []byte
	bufferIdx int
}

func NewChunkFeederWriter(size int, next ChainWriter) Interface {
	return &chunkFeeder{
		size:   size,
		next:   next,
		buffer: make([]byte, size),
	}
}

// Write assumes that the span is prepended to the actual data before the write !
func (f *chunkFeeder) Write(b []byte) (int, error) {
	l := len(b) // data length
	w := 0      // written

	if l+f.bufferIdx < f.size {
		// write the data into the buffer and return
		n := copy(f.buffer[f.bufferIdx:], b)
		f.bufferIdx += n
		return n, nil
	}

	// if we are here it means we have to do at least one write
	d := make([]byte, f.size+span)
	sp := 0 // span of current write

	//copy from existing buffer to this one
	sp = copy(d[span:], f.buffer[:f.bufferIdx])

	// don't account what was already in the buffer when returning
	// number of written bytes
	if sp > 0 {
		w -= sp
	}

	var n int
	for i := 0; i < len(b); {
		// if we can't fill a whole write, buffer the rest and return
		if sp+len(b[i:]) < f.size {
			n = copy(f.buffer, b[i:])
			f.bufferIdx = n
			return w + n, nil
		}

		// fill stuff up from the incoming write
		n = copy(d[span+f.bufferIdx:], b[i:])
		i += n
		sp += n

		binary.LittleEndian.PutUint64(d[:span], uint64(sp))
		args := &pipeWriteArgs{data: d[:span+sp]}
		_, err := f.next.ChainWrite(args)
		if err != nil {
			return 0, err
		}
		f.bufferIdx = 0
		w += sp
		sp = 0
	}
	return w, nil
}

func (f *chunkFeeder) Sum() ([]byte, error) {
	// flush existing data in the buffer
	if f.bufferIdx > 0 {
		d := make([]byte, f.bufferIdx+span)
		copy(d[span:], f.buffer[:f.bufferIdx])
		binary.LittleEndian.PutUint64(d[:span], uint64(f.bufferIdx))
		args := &pipeWriteArgs{data: d}
		_, err := f.next.ChainWrite(args)
		if err != nil {
			return nil, err
		}
	}

	return f.next.Sum()
}
