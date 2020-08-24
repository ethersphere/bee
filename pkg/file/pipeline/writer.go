package pipeline

import "io"

type ChainWriter interface {
	ChainWrite(*pipeWriteArgs) (int, error)
	Sum() ([]byte, error)
}

type Interface interface {
	io.Writer
	Sum() ([]byte, error)
}
