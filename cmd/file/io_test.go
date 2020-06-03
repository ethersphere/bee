// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/api"
	cmdfile "github.com/ethersphere/bee/cmd/file"
)

const (
	hashOfFoo = "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
)

func TestApiStore(t *testing.T) {
	storer := mock.NewStorer()
	ctx := context.Background()
	srv, addr := newTestServer(t, storer)
	defer srv.Shutdown(ctx)

	srvUrl, err := url.Parse("http://" + addr.String())
	if err != nil {
		t.Fatal(err)
	}
	srvHost := srvUrl.Hostname()
	srvPort, err := strconv.Atoi(srvUrl.Port())
	if err != nil {
		t.Fatal(err)
	}
	a := cmdfile.NewApiStore(srvHost, srvPort, false)
	t.Log(a)

	chunkAddr := swarm.MustParseHexAddress(hashOfFoo)
	chunkData := []byte("foo")
	ch := swarm.NewChunk(chunkAddr, chunkData)
	_, err = a.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = storer.Get(ctx, storage.ModeGetRequest, chunkAddr)
	if err != nil {
		t.Fatal(err)
	}
	chResult, err := a.Get(ctx, storage.ModeGetRequest, chunkAddr)
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Equal(chResult) {
		t.Fatal("chunk mismatch")
	}

}

func TestFsStore(t *testing.T) {
	tmpPath, err := ioutil.TempDir("", "cli-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpPath)
	storer := cmdfile.NewFsStore(tmpPath)
	chunkAddr := swarm.MustParseHexAddress(hashOfFoo)
	chunkData := []byte("foo")
	ch := swarm.NewChunk(chunkAddr, chunkData)

	ctx := context.Background()
	_, err = storer.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	chunkFilename := filepath.Join(tmpPath, hashOfFoo)
	chunkDataResult, err := ioutil.ReadFile(chunkFilename)
	if err != nil {
		t.Fatal(err)
	}

	chResult := swarm.NewChunk(chunkAddr, chunkDataResult)
	if !ch.Equal(chResult) {
		t.Fatal("chunk mismatch")
	}
}


func TestTeeStore(t *testing.T) {
	storeFee := mock.NewStorer()
	storeFi := mock.NewStorer()
	storeFo := mock.NewStorer()
	storeFum := cmdfile.NewTeeStore()
	storeFum.Add(storeFee)
	storeFum.Add(storeFi)
	storeFum.Add(storeFo)

	chunkAddr := swarm.MustParseHexAddress(hashOfFoo)
	chunkData := []byte("foo")
	ch := swarm.NewChunk(chunkAddr, chunkData)

	ctx := context.Background()
	var err error
	_, err = storeFum.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	_, err = storeFee.Get(ctx, storage.ModeGetRequest, chunkAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = storeFi.Get(ctx, storage.ModeGetRequest, chunkAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = storeFo.Get(ctx, storage.ModeGetRequest, chunkAddr)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLimitWriter(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	data := []byte("foo")
	closer := ioutil.NopCloser(buf).Close
	w := cmdfile.NewLimitWriteCloser(buf, closer, int64(len(data)))
	c, err := w.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if c < 3 {
		t.Fatal("short write")
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("expected written data %x, got %x", data, buf.Bytes())
	}
	_, err = w.Write(data[:1])
	if err == nil {
		t.Fatal("expected overflow error")
	}
}

func newTestServer(t *testing.T, storer storage.Storer) (*http.Server, net.Addr) {
	s := api.New(api.Options{
		Storer:   storer,
		Logger:   logging.New(os.Stdout, 6),
	})
	srv := &http.Server{
		Handler: s,
	}
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		err := srv.Serve(l)
		if err != nil {
			t.Log(err)
		}
	}()
	return srv, l.Addr()
}
