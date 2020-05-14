// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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

	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

var (
	outdir      string // flag variable, output dir for fsStore
	inputLength int64  // flag variable, limit of data input
	host        string // flag variable, http api host
	port        int    // flag variable, http api port
	noHttp      bool   // flag variable, skips http api if set
	ssl         bool   // flag variable, uses https for api if set
)

// teeStore provides a storage.Putter that can put to multiple underlying storage.Putters
type teeStore struct {
	putters []storage.Putter
}

// newTeeStore creates a new teeStore
func newTeeStore() *teeStore {
	return &teeStore{}
}

// Add adds a storage.Putter
func (t *teeStore) Add(putter storage.Putter) {
	t.putters = append(t.putters, putter)
}

// Put implements storage.Putter
func (t *teeStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, putter := range t.putters {
		_, err := putter.Put(ctx, mode, chs...)
		if err != nil {
			return []bool{}, err
		}
	}
	return []bool{}, nil
}

// fsStore provides a storage.Putter that writes chunks directly to the filesystem.
// Each chunk is a separate file, where the hex address of the chunk is the file name.
type fsStore struct {
	path string
}

// newFsStore creates a new fsStore
func newFsStore(path string) storage.Putter {
	return &fsStore{
		path: path,
	}
}

// Put implements storage.Putter
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

// apiStore provies a storage.Putter that adds chunks to swarm through the HTTP chunk API.
type apiStore struct {
	baseUrl string
}

// newApiStore creates a new apiStor
func newApiStore(host string, port int, ssl bool) (storage.Putter, error) {
	scheme := "http"
	if ssl {
		scheme += "s"
	}
	u := &url.URL{
		Host:   fmt.Sprintf("%s:%d", host, port),
		Scheme: scheme,
		Path:   "bzz-chunk",
	}
	return &apiStore{
		baseUrl: u.String(),
	}, nil
}

// Put implements storage.Putter
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

// limitReadCloser wraps the input to the application to limit the input to the given count flag.
type limitReadCloser struct {
	io.Reader
	closeFunc func() error
}

// newLimitReadCloser creates a new limitReadCloser.
func newLimitReadCloser(r io.Reader, closeFunc func() error, c int64) io.ReadCloser {
	return &limitReadCloser{
		Reader:    io.LimitReader(r, c),
		closeFunc: closeFunc,
	}
}

// Close implements io.Closer
func (l *limitReadCloser) Close() error {
	return l.closeFunc()
}

// Split is the underlying procedure for the CLI command
func Split(cmd *cobra.Command, args []string) (err error) {
	var infile io.ReadCloser

	// if one arg is set, this is the input file
	// if not, we are reading from standard input
	if len(args) > 0 {

		// get the file length
		info, err := os.Stat(args[0])
		if err != nil {
			return err
		}
		fileLength := info.Size()

		// check if we are limiting the input, and if the limit is valid
		if inputLength > 0 {
			if inputLength > fileLength {
				return fmt.Errorf("input data length set to %d on file with length %d", inputLength, fileLength)
			}
		} else {
			inputLength = fileLength
		}

		// open file and wrap in limiter
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		infile = newLimitReadCloser(f, f.Close, inputLength)
	} else {
		// this simple splitter is too stupid to handle open-ended input, sadly
		if inputLength == 0 {
			return errors.New("must specify length of input on stdin")
		}
		infile = newLimitReadCloser(os.Stdin, func() error { return nil }, inputLength)
	}

	// add the fsStore and/or apiStore, depending on flags
	stores := newTeeStore()
	if outdir != "" {
		err := os.MkdirAll(outdir, 0750)
		if err != nil {
			return err
		}
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

	// split and rule
	s := splitter.NewSimpleSplitter(stores)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()
	addr, err := s.Split(ctx, infile, inputLength)
	if err != nil {
		return err
	}

	// output the resulting hash
	fmt.Println(addr)
	return nil
}

func main() {
	c := &cobra.Command{
		Use:   "split [datafile]",
		Args:  cobra.RangeArgs(0, 1),
		Short: "Split data into swarm chunks",
		Long: `Creates and stores Swarm chunks from input data.

If datafile is not given, data will be read from standard in. In this case the --count flag must be set 
to the length of the input.

The application will expect to transmit the chunks to the bee HTTP API, unless the --no-http flag has been set.

If --output-dir is set, the chunks will be saved to the file system, using the flag argument as destination directory. 
Chunks are saved in individual files, and the file names will be the hex addresses of the chunks.`,
		RunE: Split,
	}

	c.Flags().StringVar(&outdir, "output-dir", "", "saves chunks to given directory")
	c.Flags().Int64Var(&inputLength, "count", 0, "read at most this many bytes")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "api host")
	c.Flags().IntVar(&port, "port", 8500, "api port")
	c.Flags().BoolVar(&ssl, "ssl", false, "use ssl")
	c.Flags().BoolVar(&noHttp, "no-http", false, "skip http put")
	err := c.Execute()
	if err != nil {
		os.Exit(1)
	}
}
