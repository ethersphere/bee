package pipeline

import "errors"

type resultWriter struct {
	target *pipeWriteArgs
}

func NewResultWriter(b *pipeWriteArgs) ChainableWriter {
	return &resultWriter{target: b}
}

func (w *resultWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	*w.target = *p
	return 0, nil
}

func (w *resultWriter) Sum() ([]byte, error) {
	return nil, errors.New("not implemented")
}
