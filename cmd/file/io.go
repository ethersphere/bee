// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// putGetter wraps both storage.Putter and storage.Getter interfaces
type putGetter interface {
	storage.Putter
	storage.Getter
}

// TeeStore provides a storage.Putter that can put to multiple underlying storage.Putters.
type TeeStore struct {
	putters []storage.Putter
}

// NewTeeStore creates a new TeeStore.
func NewTeeStore() *TeeStore {
	return &TeeStore{}
}

// Add adds a storage.Putter.
func (t *TeeStore) Add(putter storage.Putter) {
	t.putters = append(t.putters, putter)
}

// Put implements storage.Putter.
func (t *TeeStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, putter := range t.putters {
		_, err := putter.Put(ctx, mode, chs...)
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// FsStore provides a storage.Putter that writes chunks directly to the filesystem.
// Each chunk is a separate file, where the hex address of the chunk is the file name.
type FsStore struct {
	path string
}

// NewFsStore creates a new FsStore.
func NewFsStore(path string) storage.Putter {
	return &FsStore{
		path: path,
	}
}

// Put implements storage.Putter.
func (f *FsStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, ch := range chs {
		chunkPath := filepath.Join(f.path, ch.Address().String())
		err := ioutil.WriteFile(chunkPath, ch.Data(), 0o666)
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// ApiStore provies a storage.Putter that adds chunks to swarm through the HTTP chunk API.
type ApiStore struct {
	Client *http.Client
	baseUrl string
}

// NewApiStore creates a new ApiStore.
func NewApiStore(host string, port int, ssl bool) putGetter {
	scheme := "http"
	if ssl {
		scheme += "s"
	}
	u := &url.URL{
		Host:   fmt.Sprintf("%s:%d", host, port),
		Scheme: scheme,
		Path:   "bzz-chunk",
	}
	return &ApiStore{
		baseUrl: u.String(),
	}
}

// Put implements storage.Putter.
func (a *ApiStore) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
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

// Get implements storage.Getter.
func (a *ApiStore) Get(ctx context.Context, mode storage.ModeGet, address swarm.Address) (ch swarm.Chunk, err error) {
	addressHex := address.String()
	url := strings.Join([]string{a.baseUrl, addressHex}, "/")
	res, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chunk %s not found", addressHex)
	}
	chunkData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	ch = swarm.NewChunk(address, chunkData)
	return ch, nil
}

// LimitReadCloser wraps the input to the application to limit the input to the given count flag.
type LimitReadCloser struct {
	io.Reader
	closeFunc func() error
}

// NewLimitReadCloser creates a new LimitReadCloser.
func NewLimitReadCloser(r io.Reader, closeFunc func() error, c int64) io.ReadCloser {
	return &LimitReadCloser{
		Reader:    io.LimitReader(r, c),
		closeFunc: closeFunc,
	}
}

// Close implements io.Closer.
func (l *LimitReadCloser) Close() error {
	return l.closeFunc()
}

// LimitWriteCloser limits the output from the application.
type LimitWriteCloser struct {
	total     int64
	limit     int64
	writer    io.Writer
	closeFunc func() error
}

// NewLimitWriteCloser creates a new LimitWriteCloser.
func NewLimitWriteCloser(w io.Writer, closeFunc func() error, c int64) io.WriteCloser {
	return &LimitWriteCloser{
		limit:     c,
		writer:    w,
		closeFunc: closeFunc,
	}
}

// Write implements io.Writer.
func (l *LimitWriteCloser) Write(b []byte) (int, error) {
	if l.total+int64(len(b)) > l.limit {
		return 0, errors.New("overflow")
	}
	c, err := l.writer.Write(b)
	l.total += int64(c)
	return c, err
}

// Close implements io.Closer.
func (l *LimitWriteCloser) Close() error {
	return l.closeFunc()
}

// Join reads all output from the provided joiner.
func JoinReadAll(j file.Joiner, addr swarm.Address, outFile io.Writer) error {
	r, l, err := j.Join(context.Background(), addr)
	if err != nil {
		return err
	}
	// join, rinse, repeat until done
	data := make([]byte, swarm.ChunkSize)
	var total int64
	for i := int64(0); i < l; i += swarm.ChunkSize {
		cr, err := r.Read(data)
		if err != nil {
			return err
		}
		total += int64(cr)
		cw, err := outFile.Write(data[:cr])
		if err != nil {
			return err
		}
		if cw != cr {
			return fmt.Errorf("short wrote %d of %d for chunk %d", cw, cr, i)
		}
	}
	if total != l {
		return fmt.Errorf("received only %d of %d total bytes", total, l)
	}
	return err
}
