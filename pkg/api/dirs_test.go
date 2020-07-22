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

type dirTestCase struct {
	expectedHash string
	files        []dirTestCaseFile
}

type dirTestCaseFile struct {
	data      []byte
	name      string
	reference swarm.Address
	headers   http.Header
}

func TestDirs(t *testing.T) {
	var (
		dirUploadResource    = "/dirs"
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

	// auxiliary http.Header vars for response verification
	pngHeader := http.Header{}
	pngHeader.Set("Content-Type", "image/png")
	jpgHeader := http.Header{}
	jpgHeader.Set("Content-Type", "image/jpeg")

	for _, tc := range []dirTestCase{
		{
			expectedHash: "23bae268691842905a5273acf489d1383d1da5987b0179cf94e256814064aa63",
			files: []dirTestCaseFile{
				{
					data:      []byte("first file data"),
					name:      "img1.png",
					reference: swarm.MustParseHexAddress("e0f3cb26ab1559e6734794134cf04dd8b836342836d46178813cdb16272da7c1"),
					headers:   pngHeader,
				},
				{
					data:      []byte("second file data"),
					name:      "img2.jpg",
					reference: swarm.MustParseHexAddress("acf73b043a413e6bbccea5175a99a4577c4f6b5933c4267a1259aefea7658a1d"),
					headers:   jpgHeader,
				},
			},
		},
	} {
		t.Run("valid tar", func(t *testing.T) {
			// create and collect all files in the test case
			dirFiles := make([]*os.File, len(tc.files))
			for i, file := range tc.files {
				f := writeAndOpenFile(t, file.name, file.data)
				defer os.Remove(f.Name())
				defer f.Close()
				dirFiles[i] = f
			}

			// tar all the test case files
			tarReader := tarFiles(t, dirFiles)

			// verify directory tar upload response
			jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, tarReader, http.StatusOK, api.DirUploadResponse{
				Reference: swarm.MustParseHexAddress(tc.expectedHash),
			}, nil)

			// create expected manifest
			expectedManifest := jsonmanifest.NewManifest()
			for _, file := range tc.files {
				e := &jsonmanifest.JSONEntry{
					Reference: file.reference,
					Name:      file.name,
					Headers:   file.headers,
				}
				expectedManifest.Add(file.name, e)
			}

			b, err := expectedManifest.Serialize()
			if err != nil {
				t.Fatal(err)
			}

			// verify directory upload manifest through files api
			jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, fileDownloadResource(tc.expectedHash), nil, http.StatusOK, b, nil)
		})
	}
}

// writeAndOpenFile creates a new file with the given name and data and returns it as a variable ready for reading
// callers should make sure to close and delete the created file
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

// tarFiles receives an array of files and creates a new tar with those files as a collection
// it returns a bytes.Buffer which can be used to read the created tar
func tarFiles(t *testing.T, files []*os.File) *bytes.Buffer {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, file := range files {
		// get file info
		info, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}

		// create tar header and write it
		hdr := &tar.Header{
			Name: info.Name(),
			Mode: 0600,
			Size: info.Size(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}

		// open and read the file data
		r, err := os.Open(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		fileData, err := ioutil.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}

		// write the file data to the tar
		if _, err := tw.Write(fileData); err != nil {
			t.Fatal(err)
		}
	}

	// finally close the tar writer
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	return &buf
}
