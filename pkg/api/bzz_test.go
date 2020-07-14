// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	mmock "github.com/ethersphere/bee/pkg/manifest/mock"
	smock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestBzz(t *testing.T) {
	var (
		bzzDownloadResource = func(addr, path string) string { return "/bzz:/" + addr + "/" + path }
		storer              = smock.NewStorer()
		sp                  = splitter.NewSimpleSplitter(storer)
		client              = newTestServer(t, testServerOptions{
			Storer:         storer,
			ManifestParser: mmock.NewManifestParser(),
			Tags:           tags.NewTags(),
			Logger:         logging.New(ioutil.Discard, 5),
		})
	)

	t.Run("download-file-by-path", func(t *testing.T) {
		fileName := "sample.html"
		filePath := "test/" + fileName
		missingFilePath := "test/missing"
		sampleHtml := `<!DOCTYPE html>
		<html>
		<body>

		<h1>My First Heading</h1>

		<p>My first paragraph.</p>

		</body>
		</html>`

		var err error
		var fileReference swarm.Address
		var manifestFileReference swarm.Address

		t.Run("save-file", func(t *testing.T) {
			fileReference, err = file.SplitWriteAll(context.Background(), sp, strings.NewReader(sampleHtml), int64(len(sampleHtml)), false)
			if err != nil {
				t.Fatal(err)
			}
		})

		t.Run("save-manifest", func(t *testing.T) {

			mockManifestInterface := mmock.NewManifestInterface(map[string]manifest.Entry{
				filePath: {
					Address:  fileReference,
					Filename: fileName,
					MimeType: "text/html; charset=utf-8",
				},
			})

			manifestFileBytes := mockManifestInterface.Serialize()

			fr, err := file.SplitWriteAll(context.Background(), sp, bytes.NewReader(manifestFileBytes), int64(len(manifestFileBytes)), false)
			if err != nil {
				t.Fatal(err)
			}

			m := entry.NewMetadata(fileName)
			m.MimeType = api.ManifestType
			metadataBytes, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}

			mr, err := file.SplitWriteAll(context.Background(), sp, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), false)
			if err != nil {
				t.Fatal(err)
			}

			// now join both references (mr,fr) to create an entry and store it.
			entrie := entry.New(fr, mr)
			fileEntryBytes, err := entrie.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}

			manifestFileReference, err = file.SplitWriteAll(context.Background(), sp, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)), false)
			if err != nil {
				t.Fatal(err)
			}

		})

		t.Run("read-file-from-manifest-path", func(t *testing.T) {
			rcvdHeader := jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, bzzDownloadResource(manifestFileReference.String(), filePath), nil, http.StatusOK, []byte(sampleHtml), nil)
			cd := rcvdHeader.Get("Content-Disposition")
			_, params, err := mime.ParseMediaType(cd)
			if err != nil {
				t.Fatal(err)
			}
			if params["filename"] != fileName {
				t.Fatal("Invalid file name detected")
			}
			if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", fileReference) {
				t.Fatal("Invalid ETags header received")
			}
			if rcvdHeader.Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatal("Invalid content type detected")
			}
		})

		t.Run("invalid-path", func(t *testing.T) {
			jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodGet, bzzDownloadResource(manifestFileReference.String(), missingFilePath), nil, http.StatusBadRequest, jsonhttp.StatusResponse{
				Message: "invalid path address",
				Code:    http.StatusBadRequest,
			}, nil)
		})

	})

}
