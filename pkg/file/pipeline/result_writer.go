package pipeline

type resultWriter struct {
	target *pipeWriteArgs //the byte slice to write into
}

func NewResultWriter(b *pipeWriteArgs) ChainableWriter {
	return &resultWriter{target: b}
}

func (w *resultWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	*w.target = *p
	return 0, nil
}
