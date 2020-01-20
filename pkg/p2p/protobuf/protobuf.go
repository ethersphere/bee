package protobuf

import (
	ggio "github.com/gogo/protobuf/io"
	"github.com/janos/bee/pkg/p2p"
	"io"
)

const delimitedReaderMaxSize = 128 * 1024 // max message size

func NewWriterAndReader(s p2p.Stream) (w ggio.Writer, r ggio.Reader) {
	r = ggio.NewDelimitedReader(s, delimitedReaderMaxSize)
	w = ggio.NewDelimitedWriter(s)
	return w, r
}

func NewReader(r io.Reader) ggio.Reader {
	return ggio.NewDelimitedReader(r, delimitedReaderMaxSize)
}

func NewWriter(w io.Writer) ggio.Writer {
	return ggio.NewDelimitedWriter(w)
}
