package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/file/splitter"
)

var (
	notImplementedError = errors.New("method not implemented")
)


type fsStore struct {
	path string
}

func newFsStore(path string) *fsStore {
	return &fsStore{
		path: path,
	}
}

func (f *fsStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, ch := range chs {
		chunkPath := filepath.Join(f.path, ch.Address().String())
		err := ioutil.WriteFile(chunkPath, ch.Data(), 0o777)
		if err != nil {
			return []bool{}, err
		}
	}
	return []bool{}, nil
}

func main() {
	var infile *os.File
	var infileLength int64
	var outdir string

	if len(os.Args) > 1 {
		info, err := os.Stat(os.Args[1])
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		infileLength = info.Size()
		infile, err = os.Open(os.Args[1])
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
	} else {
		panic("cant before count is set")
		infile = os.Stdin
	}

	outdir = "chunk"
	err := os.MkdirAll(outdir, 0o777)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}

	store := newFsStore(outdir)
	s := splitter.NewSimpleSplitter(store)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second * 60)
	defer cancel()
	addr, err := s.Split(ctx, infile, infileLength)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println(addr)
}
