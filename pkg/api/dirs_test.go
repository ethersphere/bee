// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
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

	t.Run("non-tar", func(t *testing.T) {
		d := []byte("text")
		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, bytes.NewReader(d), http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "could not store dir",
			Code:    http.StatusInternalServerError,
		}, nil)
	})
}
