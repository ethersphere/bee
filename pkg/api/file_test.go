// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestFiles(t *testing.T) {
	var (
		fileUploadResource   = "/files"
		fileDownloadResource = func(addr string) string { return "/files/" + addr }
		simpleData           = []byte("this is a simple text")
		client               = newTestServer(t, testServerOptions{
			Storer: mock.NewStorer(),
			Tags:   tags.NewTags(),
			Logger: logging.New(ioutil.Discard, 5),
		})
	)

	t.Run("invalid-content-type", func(t *testing.T) {
		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, fileUploadResource, bytes.NewReader(simpleData), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "invalid content-type header",
			Code:    http.StatusBadRequest,
		}, nil)
	})

	t.Run("multipart-upload", func(t *testing.T) {
		fileName := "simple_file.txt"
		rootHash := "295673cf7aa55d119dd6f82528c91d45b53dd63dc2e4ca4abf4ed8b3a0788085"
		_ = jsonhttptest.ResponseDirectWithMultiPart(t, client, http.MethodPost, fileUploadResource, fileName, simpleData, http.StatusOK, "", api.FileUploadResponse{
			Reference: swarm.MustParseHexAddress(rootHash),
		})
	})

	t.Run("encrypt-decrypt", func(t *testing.T) {
		fileName := "my-pictures.jpeg"
		headers := make(http.Header)
		headers.Add(api.EncryptHeader, "True")
		headers.Add("Content-Type", "image/jpeg; charset=utf-8")

		_, respBytes := jsonhttptest.ResponseDirectSendHeadersAndDontCheckResponse(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, bytes.NewReader(simpleData), http.StatusOK, headers)
		read := bytes.NewReader(respBytes)

		// get the reference as everytime it will change because of random encryption key
		var resp api.FileUploadResponse
		err := json.NewDecoder(read).Decode(&resp)
		if err != nil {
			t.Fatal(err)
		}

		rootHash := resp.Reference.String()
		rcvdHeader := jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, fileDownloadResource(rootHash), nil, http.StatusOK, simpleData, nil)
		cd := rcvdHeader.Get("Content-Disposition")
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			t.Fatal(err)
		}
		if params["filename"] != fileName {
			t.Fatal("Invalid file name detected")
		}
		if rcvdHeader.Get("Content-Type") != "image/jpeg; charset=utf-8" {
			t.Fatal("Invalid content type detected")
		}
	})

	t.Run("check-content-type-detection", func(t *testing.T) {
		fileName := "my-pictures.jpeg"
		rootHash := "f2e761160deda91c1fbfab065a5abf530b0766b3e102b51fbd626ba37c3bc581"

		t.Run("binary", func(t *testing.T) {
			headers := make(http.Header)
			headers.Add("Content-Type", "image/jpeg; charset=utf-8")

			_ = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, bytes.NewReader(simpleData), http.StatusOK, api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}, headers)

			rcvdHeader := jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, fileDownloadResource(rootHash), nil, http.StatusOK, simpleData, nil)
			cd := rcvdHeader.Get("Content-Disposition")
			_, params, err := mime.ParseMediaType(cd)
			if err != nil {
				t.Fatal(err)
			}
			if params["filename"] != fileName {
				t.Fatal("Invalid file name detected")
			}
			if rcvdHeader.Get("Content-Type") != "image/jpeg; charset=utf-8" {
				t.Fatal("Invalid content type detected")
			}
		})

		t.Run("multipart", func(t *testing.T) {
			_ = jsonhttptest.ResponseDirectWithMultiPart(t, client, http.MethodPost, fileUploadResource, fileName, simpleData, http.StatusOK, "image/jpeg; charset=utf-8", api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			})

			rcvdHeader := jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, fileDownloadResource(rootHash), nil, http.StatusOK, simpleData, nil)
			cd := rcvdHeader.Get("Content-Disposition")
			_, params, err := mime.ParseMediaType(cd)
			if err != nil {
				t.Fatal(err)
			}
			if params["filename"] != fileName {
				t.Fatal("Invalid file name detected")
			}
			if rcvdHeader.Get("Content-Type") != "image/jpeg; charset=utf-8" {
				t.Fatal("Invalid content type detected")
			}
		})
	})

	t.Run("upload-then-download-and-check-data", func(t *testing.T) {
		fileName := "sample.html"
		rootHash := "9f8ba407ff4809e877c75506247e0f1faf206262d1ddd7b3c8f9775d3501be50"
		sampleHtml := `<!DOCTYPE html>
		<html>
		<body>

		<h1>My First Heading</h1>

		<p>My first paragraph.</p>

		</body>
		</html>`

		t.Run("binary", func(t *testing.T) {
			headers := make(http.Header)
			headers.Add("Content-Type", "text/html; charset=utf-8")

			rcvdHeader := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, strings.NewReader(sampleHtml), http.StatusOK, api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}, headers)

			if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", rootHash) {
				t.Fatal("Invalid ETags header received")
			}

			// try to fetch the same file and check the data
			rcvdHeader = jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, fileDownloadResource(rootHash), nil, http.StatusOK, []byte(sampleHtml), nil)

			// check the headers
			cd := rcvdHeader.Get("Content-Disposition")
			_, params, err := mime.ParseMediaType(cd)
			if err != nil {
				t.Fatal(err)
			}
			if params["filename"] != fileName {
				t.Fatal("Invalid filename detected")
			}
			if rcvdHeader.Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatal("Invalid content type detected")
			}
		})

		t.Run("multipart", func(t *testing.T) {
			rcvdHeader := jsonhttptest.ResponseDirectWithMultiPart(t, client, http.MethodPost, fileUploadResource, fileName, []byte(sampleHtml), http.StatusOK, "", api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			})

			if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", rootHash) {
				t.Fatal("Invalid ETags header received")
			}

			// try to fetch the same file and check the data
			rcvdHeader = jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, fileDownloadResource(rootHash), nil, http.StatusOK, []byte(sampleHtml), nil)

			// check the headers
			cd := rcvdHeader.Get("Content-Disposition")
			_, params, err := mime.ParseMediaType(cd)
			if err != nil {
				t.Fatal(err)
			}
			if params["filename"] != fileName {
				t.Fatal("Invalid filename detected")
			}
			if rcvdHeader.Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatal("Invalid content type detected")
			}
		})
	})

}
