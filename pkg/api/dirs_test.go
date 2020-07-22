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

	t.Run("valid tar", func(t *testing.T) {
		//f1 := tempFile(t, "valid-tar", []byte("first file data"))
		//defer os.Remove(f1.Name())
		//f2 := tempFile(t, "valid-tar", []byte("second file data"))
		//defer os.Remove(f2.Name())

		f1 := "file-1"
		ioutil.WriteFile(f1, []byte("first file data"), 0755)
		defer os.Remove(f1)
		file1, err := os.Open(f1)
		if err != nil {
			t.Fatal(err)
		}
		f2 := "file-2"
		ioutil.WriteFile(f2, []byte("second file data"), 0755)
		defer os.Remove(f2)
		file2, err := os.Open(f2)
		if err != nil {
			t.Fatal(err)
		}

		files := make([]*os.File, 2)
		files[0] = file1
		files[1] = file2

		buf := tarFiles(t, files)

		expectedHash := "e65f543aff65e43b48f3741531142f1c920990ded97075de8f4e5fa3f3e2cb21"

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, &buf, http.StatusOK, api.DirUploadResponse{
			Reference: swarm.MustParseHexAddress(expectedHash),
		}, nil)

		m := jsonmanifest.NewManifest()
		h := http.Header{}
		h.Set("Content-Type", "")
		e := &jsonmanifest.JSONEntry{
			Reference: swarm.MustParseHexAddress("def0d29fe7bd77da42490de711bb139f4fe16fc191f1df566f3e5db7371ad8d7"),
			Name:      f1,
			Headers:   h,
		}
		m.Add(f1, e)
		e = &jsonmanifest.JSONEntry{
			Reference: swarm.MustParseHexAddress("beda80b55870e556f39892f146badaf8b80e4732b49d22444313aef0cd5029d9"),
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
