package pipeline

type resultWriter struct {
	target *pipeWriteArgs //the byte slice to write into
}

func NewResultWriter(b []byte) ChainableWriter {
	return &resultWriter{target: b}
}

func (w *resultWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	*w.target = *p
	return 0, nil
}
