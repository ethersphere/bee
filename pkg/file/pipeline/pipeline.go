package pipeline

import "io"

type pipeWriteArgs struct {
	ref  []byte
	key  []byte
	span []byte
	data []byte //data includes the span too!
}

type Pipeline struct {
	head io.Writer
	tail EndPipeWriter
}

func (p *Pipeline) Write(b []byte) (int, error) {
	return p.head.Write(b)
}

func (p *Pipeline) Sum() ([]byte, error) {
	return p.tail.Sum()
}

func NewPipeline() EndPipeWriter {
	tw := NewHashTrieWriter(4096, 128, 32)
	lsw := NewStoreWriter(nil, tw)
	b := NewBmtWriter(128, lsw)

	tw.SetHead(b) //allow reuse of the pipeline

	return &Pipeline{head: b, tail: tw}
}

func NewShortPipeline(b []byte) io.Writer {
	rsw := NewResultWriter(b)
	lsw := NewStoreWriter(nil, tw)
	bw := NewBmtWriter(128, lsw)

	return bw
}

//func NewEncryptionPipeline() EndPipeWriter {
//tw := NewHashTrieWriter()
//lsw := NewStoreWriter()
//b := NewEncryptingBmtWriter(128, lsw) // needs to pass the key somehwoe...
//enc := NewEncryptionWriter(b)
//return
//}
