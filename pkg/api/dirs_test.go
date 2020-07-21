// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestDirs(t *testing.T) {
	var (
		dirUploadResource = "/dirs"
		//dirDownloadResource = func(addr string) string { return "/dirs/" + addr }
		client = newTestServer(t, testServerOptions{
			Storer: mock.NewStorer(),
			Tags:   tags.NewTags(),
			Logger: logging.New(ioutil.Discard, 5),
		})
	)

	t.Run("empty request body", func(t *testing.T) {
		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, bytes.NewReader(nil), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "could not extract dir info from request",
			Code:    http.StatusBadRequest,
		}, nil)
	})

	t.Run("empty file", func(t *testing.T) {
		f := tempFile(t, "empty", nil)
		defer os.Remove(f.Name())

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, getFileReader(t, f), http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "could not store dir",
			Code:    http.StatusInternalServerError,
		}, nil)
	})

	t.Run("non tar file", func(t *testing.T) {
		f := tempFile(t, "non-tar", []byte("some data"))
		defer os.Remove(f.Name())

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, getFileReader(t, f), http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "could not store dir",
			Code:    http.StatusInternalServerError,
		}, nil)
	})
}

func tempFile(t *testing.T, name string, data []byte) *os.File {
	f, err := ioutil.TempFile("", name)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	return f
}

func getFileReader(t *testing.T, f *os.File) *os.File {
	r, err := os.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return r
}
