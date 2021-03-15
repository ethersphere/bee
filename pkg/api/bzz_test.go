// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	smock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestBzz(t *testing.T) {
	var (
		bzzDownloadResource = func(addr, path string) string { return "/bzz/" + addr + "/" + path }
		storer              = smock.NewStorer()
		ctx                 = context.Background()
		mockStatestore      = statestore.NewStateStore()
		logger              = logging.New(ioutil.Discard, 0)
		client, _, _        = newTestServer(t, testServerOptions{
			Storer: storer,
			Tags:   tags.NewTags(mockStatestore, logger),
			Logger: logging.New(ioutil.Discard, 5),
		})
		pipeWriteAll = func(r io.Reader, l int64) (swarm.Address, error) {
			pipe := builder.NewPipelineBuilder(ctx, storer, storage.ModePutUpload, false)
			return builder.FeedPipeline(ctx, pipe, r, l)
		}
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
		var fileContentReference swarm.Address
		var fileReference swarm.Address
		var manifestFileReference swarm.Address

		// save file
		fileContentReference, err = pipeWriteAll(strings.NewReader(sampleHtml), int64(len(sampleHtml)))

		if err != nil {
			t.Fatal(err)
		}

		fileMetadata := entry.NewMetadata(fileName)
		fileMetadata.MimeType = "text/html; charset=utf-8"
		fileMetadataBytes, err := json.Marshal(fileMetadata)
		if err != nil {
			t.Fatal(err)
		}

		fileMetadataReference, err := pipeWriteAll(bytes.NewReader(fileMetadataBytes), int64(len(fileMetadataBytes)))
		if err != nil {
			t.Fatal(err)
		}

		fe := entry.New(fileContentReference, fileMetadataReference)
		fileEntryBytes, err := fe.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		fileReference, err = pipeWriteAll(bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)))

		if err != nil {
			t.Fatal(err)
		}

		// save manifest
		m, err := manifest.NewDefaultManifest(loadsave.New(storer, storage.ModePutRequest, false), false)
		if err != nil {
			t.Fatal(err)
		}

		e := manifest.NewEntry(fileReference, nil)

		err = m.Add(ctx, filePath, e)
		if err != nil {
			t.Fatal(err)
		}

		manifestBytesReference, err := m.Store(ctx)
		if err != nil {
			t.Fatal(err)
		}

		metadata := entry.NewMetadata(manifestBytesReference.String())
		metadata.MimeType = m.Type()
		metadataBytes, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}

		mr, err := pipeWriteAll(bytes.NewReader(metadataBytes), int64(len(metadataBytes)))
		if err != nil {
			t.Fatal(err)
		}

		// now join both references (fr,mr) to create an entry and store it.
		newEntry := entry.New(manifestBytesReference, mr)
		manifestFileEntryBytes, err := newEntry.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}

		manifestFileReference, err = pipeWriteAll(bytes.NewReader(manifestFileEntryBytes), int64(len(manifestFileEntryBytes)))
		if err != nil {
			t.Fatal(err)
		}

		// read file from manifest path

		rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet, bzzDownloadResource(manifestFileReference.String(), filePath), http.StatusOK,
			jsonhttptest.WithExpectedResponse([]byte(sampleHtml)),
		)
		cd := rcvdHeader.Get("Content-Disposition")
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			t.Fatal(err)
		}
		if params["filename"] != fileName {
			t.Fatal("Invalid file name detected")
		}
		if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", fileContentReference) {
			t.Fatal("Invalid ETags header received")
		}
		if rcvdHeader.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatal("Invalid content type detected")
		}

		// check on invalid path

		jsonhttptest.Request(t, client, http.MethodGet, bzzDownloadResource(manifestFileReference.String(), missingFilePath), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "path address not found",
				Code:    http.StatusNotFound,
			}),
		)
	})
}

func TestFeedIndirection(t *testing.T) {
	// first, "upload" some content for the update
	var (
		updateData     = []byte("<h1>Swarm Feeds Hello World!</h1>")
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		storer         = smock.NewStorer()
		client, _, _   = newTestServer(t, testServerOptions{
			Storer: storer,
			Tags:   tags.NewTags(mockStatestore, logger),
			Logger: logger,
		})
	)
	// tar all the test case files
	tarReader := tarFiles(t, []f{
		{
			data:     updateData,
			name:     "index.html",
			dir:      "",
			filePath: "./index.html",
		},
	})

	var resp api.FileUploadResponse

	options := []jsonhttptest.Option{
		jsonhttptest.WithRequestBody(tarReader),
		jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
		jsonhttptest.WithUnmarshalJSONResponse(&resp),
		jsonhttptest.WithRequestHeader(api.SwarmIndexDocumentHeader, "index.html"),
	}

	// verify directory tar upload response
	jsonhttptest.Request(t, client, http.MethodPost, "/dirs", http.StatusOK, options...)

	if resp.Reference.String() == "" {
		t.Fatalf("expected file reference, did not got any")
	}

	// now use the "content" to mock the feed lookup
	// also, use the mocked mantaray chunks that unmarshal
	// into a real manifest with the mocked feed values when
	// called from the bzz endpoint. then call the bzz endpoint with
	// the pregenerated feed root manifest hash

	feedUpdate := toChunk(t, 121212, resp.Reference.Bytes())

	var (
		feedChunkAddr       = swarm.MustParseHexAddress("891a1d1c8436c792d02fc2e8883fef7ab387eaeaacd25aa9f518be7be7856d54")
		feedChunkData, _    = hex.DecodeString("400100000000000000000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f200000000000000000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000012012f00000000000000000000000000000000000000000000000000000000008504f2a107ca940beafc4ce2f6c9a9f0968c62a5b5893ff0e4e1e2983048d276007e7b22737761726d2d666565642d6f776e6572223a2238643337363634343066306437623934396135653332393935643039363139613766383665363332222c22737761726d2d666565642d746f706963223a22616162626363222c22737761726d2d666565642d74797065223a2253657175656e6365227d0a0a0a0a0a0a")
		chData, _           = hex.DecodeString("800000000000000000000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f2000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
		manifestCh          = swarm.NewChunk(swarm.MustParseHexAddress("8504f2a107ca940beafc4ce2f6c9a9f0968c62a5b5893ff0e4e1e2983048d276"), chData)
		look                = newMockLookup(-1, 0, feedUpdate, nil, &id{}, nil)
		factory             = newMockFactory(look)
		bzzDownloadResource = func(addr, path string) string { return "/bzz/" + addr + "/" + path }
		ctx                 = context.Background()
	)
	client, _, _ = newTestServer(t, testServerOptions{
		Storer: storer,
		Tags:   tags.NewTags(mockStatestore, logger),
		Logger: logging.New(ioutil.Discard, 0),
		Feeds:  factory,
	})
	_, err := storer.Put(ctx, storage.ModePutUpload, swarm.NewChunk(feedChunkAddr, feedChunkData))
	if err != nil {
		t.Fatal(err)
	}
	_, err = storer.Put(ctx, storage.ModePutUpload, feedUpdate)
	if err != nil {
		t.Fatal(err)
	}
	_, err = storer.Put(ctx, storage.ModePutUpload, manifestCh)
	if err != nil {
		t.Fatal(err)
	}

	jsonhttptest.Request(t, client, http.MethodGet, bzzDownloadResource(feedChunkAddr.String(), ""), http.StatusOK,
		jsonhttptest.WithExpectedResponse(updateData),
	)
}
