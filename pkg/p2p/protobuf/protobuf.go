package protobuf

import (
	ggio "github.com/gogo/protobuf/io"
	"github.com/janos/bee/pkg/p2p"
)

const delimitedReaderMaxSize = 128 * 1024 // max message size

func NewRW(s p2p.Stream) (w ggio.Writer, r ggio.Reader) {
	r = ggio.NewDelimitedReader(s, delimitedReaderMaxSize)
	w = ggio.NewDelimitedWriter(s)
	return w, r
}
