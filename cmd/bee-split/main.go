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

	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

var (
	outdir      string // flag variable, output dir for fsStore
	inputLength int64  // flag variable, limit of data input
	host        string // flag variable, http api host
	port        int    // flag variable, http api port
	outHttp     bool   // flag variable, skips http api if set
	ssl         bool   // flag variable, uses https for api if set
	loglevel    int    // flag variable, sets loglevel for operation
	logger      logging.Logger
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
			return nil, err
		}
	}
	return nil, nil
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
		err := ioutil.WriteFile(chunkPath, ch.Data(), 0o666)
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// apiStore provies a storage.Putter that adds chunks to swarm through the HTTP chunk API.
type apiStore struct {
	baseUrl string
}

// newApiStore creates a new apiStor
func newApiStore(host string, port int, ssl bool) storage.Putter {
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
	}
}

// Put implements storage.Putter
func (a *apiStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	c := http.DefaultClient
	for _, ch := range chs {
		addr := ch.Address().String()
		buf := bytes.NewReader(ch.Data())
		url := strings.Join([]string{a.baseUrl, addr}, "/")
		res, err := c.Post(url, "application/octet-stream", buf)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("upload failed: %v", res.Status)
		}
	}
	return nil, nil
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
	loglevelConst := logging.ToLogLevel(loglevel)
	logger = logging.New(os.Stderr, loglevelConst)

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
		infile = newLimitReadCloser(f, f.Close, inputLength)
		logger.Debugf("Reading %d bytes of data from %s", inputLength, args[0])
	} else {
		// this simple splitter is too stupid to handle open-ended input, sadly
		if inputLength == 0 {
			return errors.New("must specify length of input on stdin")
		}
		infile = newLimitReadCloser(os.Stdin, func() error { return nil }, inputLength)
		logger.Debugf("Reading %d bytes of data from stdin", inputLength)
	}

	// add the fsStore and/or apiStore, depending on flags
	stores := newTeeStore()
	if outdir != "" {
		err := os.MkdirAll(outdir, 0o777) // skipcq: GSC-G301
		if err != nil {
			return err
		}
		store := newFsStore(outdir)
		stores.Add(store)
	}
	if outHttp {
		store := newApiStore(host, port, ssl)
		stores.Add(store)
	}

	// split and rule
	s := splitter.NewSimpleSplitter(stores)
	ctx, cancel := context.WithCancel(context.Background())
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
		Short: "Create Swarm Hash from input data",
		Long: `Creates a Swarm Hash file tree from input data, and optionally stores the created chunks.

If datafile is not given, data will be read from standard in. In this case the --count flag must be set 
to the length of the input.

If the --http flag has been set, the application will attempt to transmit the chunks to the bee HTTP API.

If --output-dir is set, the chunks will be saved to the file system, using the flag argument as destination directory. 
Chunks are saved in individual files, and the file names will be the hex addresses of the chunks.`,
		RunE:         Split,
		SilenceUsage: true,
	}

	c.Flags().StringVarP(&outdir, "output-dir", "d", "", "save chunks to given directory")
	c.Flags().Int64VarP(&inputLength, "count", "c", 0, "read at most this many bytes")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "api host")
	c.Flags().IntVar(&port, "port", 8080, "api port")
	c.Flags().BoolVarP(&ssl, "ssl", "s", false, "use ssl")
	c.Flags().IntVarP(&loglevel, "loglevel", "l", 0, "log verbosity")
	c.Flags().BoolVar(&outHttp, "http", false, "save chunks to given bee endpoint")
	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
