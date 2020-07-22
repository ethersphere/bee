// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest/jsonmanifest"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestDirs(t *testing.T) {
	var (
		dirUploadResource = "/dirs"

		fileDownloadResource = func(addr string) string { return "/files/" + addr }
		client               = newTestServer(t, testServerOptions{
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
		f := writeAndOpenFile(t, "empty-file", nil)
		defer os.Remove(f.Name())
		defer f.Close()

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, f, http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "could not store dir",
			Code:    http.StatusInternalServerError,
		}, nil)
	})

	t.Run("non tar file", func(t *testing.T) {
		f := writeAndOpenFile(t, "non-tar-file", []byte("some data"))
		defer os.Remove(f.Name())
		defer f.Close()

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, f, http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "could not store dir",
			Code:    http.StatusInternalServerError,
		}, nil)
	})

	t.Run("valid tar", func(t *testing.T) {
		f1 := "img1.png"
		file1 := writeAndOpenFile(t, f1, []byte("first file data"))
		defer os.Remove(file1.Name())
		defer file1.Close()

		f2 := "img2.jpg"
		ioutil.WriteFile(f2, []byte("second file data"), 0755)

		file2 := writeAndOpenFile(t, f2, []byte("second file data"))
		defer os.Remove(file2.Name())
		defer file2.Close()

		files := make([]*os.File, 2)
		files[0] = file1
		files[1] = file2

		buf := tarFiles(t, files)

		expectedHash := "23bae268691842905a5273acf489d1383d1da5987b0179cf94e256814064aa63"

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, &buf, http.StatusOK, api.DirUploadResponse{
			Reference: swarm.MustParseHexAddress(expectedHash),
		}, nil)

		m := jsonmanifest.NewManifest()
		h := http.Header{}
		h.Set("Content-Type", "image/png")
		e := &jsonmanifest.JSONEntry{
			Reference: swarm.MustParseHexAddress("e0f3cb26ab1559e6734794134cf04dd8b836342836d46178813cdb16272da7c1"),
			Name:      f1,
			Headers:   h,
		}
		m.Add(f1, e)
		h = http.Header{}
		h.Set("Content-Type", "image/jpeg")
		e = &jsonmanifest.JSONEntry{
			Reference: swarm.MustParseHexAddress("acf73b043a413e6bbccea5175a99a4577c4f6b5933c4267a1259aefea7658a1d"),
			Name:      f2,
			Headers:   h,
		}

		m.Add(f2, e)
		b, err := m.Serialize()
		if err != nil {
			t.Fatal(err)
		}

		jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, fileDownloadResource(expectedHash), nil, http.StatusOK, b, nil)
	})
}

func writeAndOpenFile(t *testing.T, name string, data []byte) *os.File {
	err := ioutil.WriteFile(name, data, 0755)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}

	return f
}

func tarFiles(t *testing.T, files []*os.File) bytes.Buffer {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, file := range files {
		info, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		hdr := &tar.Header{
			Name: info.Name(),
			Mode: 0600,
			Size: info.Size(),
		}

		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}

		r, err := os.Open(file.Name())
		if err != nil {
			t.Fatal(err)
		}

		fileData, err := ioutil.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := tw.Write(fileData); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf
}
