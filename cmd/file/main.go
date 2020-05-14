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
	"github.com/spf13/cobra"
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

func Split(cmd *cobra.Command, args []string) (err error) {
	var infile *os.File
	var infileLength int64
	var outdir string

	if len(args) > 0 {
		info, err := os.Stat(args[0])
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			return err
		}
		infileLength = info.Size()
		infile, err = os.Open(args[0])
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			return err
		}
	} else {
		panic("cant before count is set")
		infile = os.Stdin
	}

	outdir = "chunk"
	err = os.MkdirAll(outdir, 0o777)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		return err
	}

	store := newFsStore(outdir)
	s := splitter.NewSimpleSplitter(store)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second * 60)
	defer cancel()
	addr, err := s.Split(ctx, infile, infileLength)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		return err
	}
	fmt.Println(addr)
	return nil
}

func main() {
	c := &cobra.Command{
		Use: "split",
		Short: "split data into swarm chunks",
		RunE: Split,
	}
	c.Execute()
}
