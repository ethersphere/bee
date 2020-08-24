package pipeline

import (
	"encoding/binary"
	"fmt"
)

type chunkFeeder struct {
	size int
	next ChainWriter
}

func NewChunkFeederWriter(size int, next ChainWriter) Interface {
	return &chunkFeeder{
		size: size,
		next: next,
	}
}

// Write assumes that the span is prepended to the actual data before the write !
func (f *chunkFeeder) Write(b []byte) (int, error) {
	l := len(b)
	w := 0
	for i := 0; i < len(b); i += f.size {
		var d []byte
		if i+f.size > l {
			d = b[i:]
		} else {
			d = b[i : i+f.size]
		}
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data[:8], uint64(len(d)))
		data = append(data, d...)

		args := &pipeWriteArgs{data: data}
		_, err := f.next.ChainWrite(args)
		if err != nil {
			return 0, err
		}
		w += len(d)
	}
	fmt.Println(w)
	return w, nil
}

func (w *chunkFeeder) Sum() ([]byte, error) {
	return w.next.Sum()
}
