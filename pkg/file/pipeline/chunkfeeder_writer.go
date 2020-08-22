package pipeline

import (
	"io"
)

type chunkFeeder struct {
	size int
	next ChainableWriter
}

func NewChunkFeederWriter(size int, next ChainableWriter) io.Writer {
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

		args := &pipeWriteArgs{data: d}
		i, err := f.next.ChainWrite(args)
		if err != nil {
			return 0, err
		}
		w += i
	}
	return w, nil
}
