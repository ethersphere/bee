// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"context"
	"os"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/api"
	cmdfile "github.com/ethersphere/bee/cmd/file"
)

func TestApiServer(t *testing.T) {
	ctx := context.Background()
	srv, addr := newTestServer(t)
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

	chunkAddr := swarm.MustParseHexAddress("2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48")
	chunkData := []byte("foo")
	ch := swarm.NewChunk(chunkAddr, chunkData)
	a.Put(ctx, storage.ModePutUpload, ch)

}

func newTestServer(t *testing.T) (*http.Server, net.Addr) {
	storer := mock.NewStorer()
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
		t.Fatal(err)
	}
}()
	return srv, l.Addr()
}
