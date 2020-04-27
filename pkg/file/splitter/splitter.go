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
	j := internal.NewSimpleSplitterJob(ctx, r, s.store)

	_ = j
	return swarm.ZeroAddress, nil
}

