package file

import (
	"io"
)

type ChainableWriter interface {
	ChainWrite(*pipeWriteArgs)
}

// this one is by definition not chainable and is used in the end of the pipeline
// in order to execute any pending operations
type EndPipeWriter interface {
	ChainableWriter
	Sum() ([]byte, error)
	SetHead(io.Writer)
}
