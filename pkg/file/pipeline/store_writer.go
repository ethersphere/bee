package pipeline

import (
	"context"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type storeWriter struct {
	l    storage.Putter
	next ChainWriter
}

func NewStoreWriter(l storage.Putter, next ChainWriter) ChainWriter {
	return &storeWriter{l: l, next: next}
}

func (w *storeWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	c := swarm.NewChunk(swarm.NewAddress(p.ref), p.data)
	w.l.Put(context.Background(), storage.ModePutUpload, c)
	return w.next.ChainWrite(p)
}

func (w *storeWriter) Sum() ([]byte, error) {
	return w.next.Sum()
}
