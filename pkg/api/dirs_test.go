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
	"path"
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
	path      string
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

	for _, tc := range []dirTestCase{
		{
			expectedHash: "8db93fe0558b9692a24f96cec0880bc8cdad692063ba9d9db411f1c13b851f7a",
			files: []dirTestCaseFile{
				{
					data:      []byte("first file data"),
					name:      "file1",
					path:      "",
					reference: swarm.MustParseHexAddress("3c07cd2cf5c46208d69d554b038f4dce203f53ac02cb8a313a0fe1e3fe6cc3cf"),
					headers: http.Header{
						"Content-Type": {""},
					},
				},
				{
					data:      []byte("second file data"),
					name:      "file2",
					path:      "",
					reference: swarm.MustParseHexAddress("47e1a2a8f16e02da187fac791d57e6794f3e9b5d2400edd00235da749ad36683"),
					headers: http.Header{
						"Content-Type": {""},
					},
				},
			},
		},
		{
			expectedHash: "af73c63627d076ed12dda839cf236e460b797cb29ab0e20c0e453559ed37cdd3",
			files: []dirTestCaseFile{
				{
					data:      []byte("robots text"),
					name:      "robots.txt",
					path:      "",
					reference: swarm.MustParseHexAddress("17b96d0a800edca59aaf7e40c6053f7c4c0fb80dd2eb3f8663d51876bf350b12"),
					headers: http.Header{
						"Content-Type": {"text/plain; charset=utf-8"},
					},
				},
				{
					data:      []byte("image 1"),
					name:      "1.png",
					path:      "img",
					reference: swarm.MustParseHexAddress("3c1b3fc640e67f0595d9c1db23f10c7a2b0bdc9843b0e27c53e2ac2a2d6c4674"),
					headers: http.Header{
						"Content-Type": {"image/png"},
					},
				},
				{
					data:      []byte("image 2"),
					name:      "2.png",
					path:      "img",
					reference: swarm.MustParseHexAddress("b234ea7954cab7b2ccc5e07fe8487e932df11b2275db6b55afcbb7bad0be73fb"),
					headers: http.Header{
						"Content-Type": {"image/png"},
					},
				},
			},
		},
	} {
		t.Run("valid tar", func(t *testing.T) {
			// create and collect all files in the test case
			dirFiles := make([]*os.File, len(tc.files))
			for i, file := range tc.files {
				// create dir if the file is nested
				if file.path != "" {
					if _, err := os.Stat(file.path); os.IsNotExist(err) {
						if err := os.Mkdir(file.path, 0700); err != nil {
							t.Fatal(err)
						}
						defer os.RemoveAll(file.path)
					}
				}
				f := writeAndOpenFile(t, path.Join(file.path, file.name), file.data)
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
				expectedManifest.Add(path.Join(file.path, file.name), e)
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
			Name: file.Name(),
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
