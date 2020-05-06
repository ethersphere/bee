package splitter

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter/internal"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// simpleSplitter wraps a non-optimized implementation of file.Splitter
type simpleSplitter struct {
	store  storage.Storer
	logger logging.Logger
}

// NewSimpleSplitter creates a new SimpleSplitter
func NewSimpleSplitter(store storage.Storer) file.Splitter {
	return &simpleSplitter{
		store:  store,
		logger: logging.New(os.Stderr, 6),
	}
}

// Split implements the file.Splitter interface
//
// It uses a non-optimized internal component that blocks when performing
// multiple levels of hashing when building the file hash tree.
//
// It returns the Swarmhash of the data.
func (s *simpleSplitter) Split(ctx context.Context, r io.ReadCloser, dataLength int64) (addr swarm.Address, err error) {
	j := internal.NewSimpleSplitterJob(ctx, s.store, dataLength)

	var total int
	data := make([]byte, swarm.ChunkSize)
	for {
		c, err := r.Read(data)
		if err != nil {
			if err == io.EOF {
				break
			}
			return swarm.ZeroAddress, err
		}
		cc, err := j.Write(data[:c])
		if err != nil {
			return swarm.ZeroAddress, err
		}
		if cc < c {
			return swarm.ZeroAddress, fmt.Errorf("write count to file hasher component %d does not match read count %d", cc, c)
		}
		total += c
	}

	sum := j.Sum(nil)
	return swarm.NewAddress(sum), nil
}
