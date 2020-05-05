package splitter

import (
	"context"
	"io"
	"os"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter/internal"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type simpleSplitter struct {
	store storage.Storer
	logger logging.Logger
}

func NewSimpleSplitter(store storage.Storer) file.Splitter {
	return &simpleSplitter{
		store: store,
		logger: logging.New(os.Stderr, 6),
	}
}

func (s *simpleSplitter) Split(ctx context.Context, r io.ReadCloser, dataLength int64) (addr swarm.Address, err error) {
	j := internal.NewSimpleSplitterJob(ctx, s.store, dataLength)

	var total int
	data := make([]byte, swarm.ChunkSize)
	for {
		c, err := r.Read(data)
		// TODO: provide error to caller
		if err != nil {
			if err == io.EOF {
				break
			}
			return swarm.ZeroAddress, err
		}
		j.Write(data[:c])
		//j.update(0, data[:c])
		total += c
	}

	return swarm.ZeroAddress, nil
}
