package pipeline

import "io"

type ChainableWriter interface {
	ChainWrite(*pipeWriteArgs) (int, error)
	Sum() ([]byte, error)
}

// this one is by definition not chainable and is used in the end of the pipeline
// in order to execute any pending operations
type EndPipeWriter interface {
	ChainableWriter
	Sum() ([]byte, error)
}

type Interface interface {
	io.Writer
	Sum() ([]byte, error)
}
