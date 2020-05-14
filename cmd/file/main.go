package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	host string
	port int
	noHttp bool
	ssl bool
)

type teeStore struct {
	putters []storage.Putter
}

func newTeeStore() *teeStore {
	return &teeStore{}
}

func (t *teeStore) Add(putter storage.Putter) {
	t.putters = append(t.putters, putter)
}

func (t *teeStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, putter := range t.putters {
		_, err := putter.Put(ctx, mode, chs...)
		if err != nil {
			return []bool{}, err
		}
	}
	return []bool{}, nil
}

type fsStore struct {
	path string
}

func newFsStore(path string) storage.Putter {
	return &fsStore{
		path: path,
	}
}

type apiStore struct {
	baseUrl string
}

func newApiStore(host string, port int, ssl bool) (storage.Putter, error) {
	proto := "http"
	if ssl {
		proto += "s"
	}
	urlString := fmt.Sprintf("%s://%s:%d/bzz-chunk", proto, host, port)
	url, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	return &apiStore{
		baseUrl: url.String(),
	}, nil
}

func (a *apiStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, ch := range chs {
		addr := ch.Address().String()
		buf := bytes.NewReader(ch.Data())
		url := strings.Join([]string{a.baseUrl, addr}, "/")
		c := &http.Client{}
		res, err := c.Post(url, "application/octet-stream", buf)
		if err != nil {
			return []bool{}, err
		}
		_ = res
	}
	return []bool{}, nil
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

	stores := newTeeStore()
	if outdir != "" {
		store := newFsStore(outdir)
		stores.Add(store)
	}
	if !noHttp {
		store, err := newApiStore(host, port, ssl)
		if err != nil {
			return err
		}
		stores.Add(store)
	}
	s := splitter.NewSimpleSplitter(stores)
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

	c.Flags().StringVar(&outdir, "output-dir", "", "output directory")
	c.Flags().Int64Var(&inputLength, "count", 0, "input data length")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "api host")
	c.Flags().IntVar(&port, "port", 8500, "api port")
	c.Flags().BoolVar(&ssl, "ssl", false, "use ssl")
	c.Flags().BoolVar(&noHttp, "no-http", false, "skip http put")
	c.Execute()
}
