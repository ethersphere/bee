package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	outdir string
	inputLength int64
)


type fsStore struct {
	path string
}

func newFsStore(path string) *fsStore {
	return &fsStore{
		path: path,
	}
}

type limitReadCloser struct {
	io.Reader
	closeFunc func() error
}

func newLimitReadCloser(r io.Reader, closeFunc func() error, c int64) io.ReadCloser {
	return &limitReadCloser{
		Reader: io.LimitReader(r, c),
		closeFunc: closeFunc,
	}
}

func (l *limitReadCloser) Close() error {
	return l.closeFunc()
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
	var infile io.ReadCloser

	if len(args) > 0 {
		info, err := os.Stat(args[0])
		if err != nil {
			return err
		}
		fileLength := info.Size()
		if inputLength > 0 {
			if inputLength > fileLength {
				return fmt.Errorf("input data length set to %d on file with length %d", inputLength, fileLength)
			}
		} else {
			inputLength = fileLength
		}
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		infile = newLimitReadCloser(f, f.Close, inputLength)
	} else {
		if inputLength == 0 {
			return errors.New("must specify length of input on stdin")
		}
		infile = newLimitReadCloser(os.Stdin, func() error { return nil }, inputLength)
	}

	err = os.MkdirAll(outdir, 0o777)
	if err != nil {
		return err
	}

	store := newFsStore(outdir)
	s := splitter.NewSimpleSplitter(store)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second * 60)
	defer cancel()
	addr, err := s.Split(ctx, infile, inputLength)
	if err != nil {
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

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
	c.Flags().StringVar(&outdir, "output-dir", filepath.Join(dir, "chunks"), "output directory")
	c.Flags().Int64Var(&inputLength, "count", 0, "input data length")
	c.Execute()
}
