// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"encoding/hex"
	"fmt"
	// "io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	smock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

var feedChunkStr = `
400100000000000000000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f200000000000000000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000012012f00000000000000000000000000000000000000000000000000000000008504f2a107ca940beafc4ce2f6c9a9f0968c62a5b5893ff0e4e1e2983048d276007e7b22737761726d2d666565642d6f776e6572223a2238643337363634343066306437623934396135653332393935643039363139613766383665363332222c22737761726d2d666565642d746f706963223a22616162626363222c22737761726d2d666565642d74797065223a2253657175656e6365227d0a0a0a0a0a0a
`

var chunkDataStr = `
800000000000000000000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f2000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000
`

func TestFeedIndirection(t *testing.T) {
	// first, "upload" some content for the update
	var (
		updateData     = []byte("<h1>Swarm Feeds Hello World!</h1>")
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(os.Stdout, 6)
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
		feedChunkData, _    = hex.DecodeString(feedChunkStr)
		chData, _           = hex.DecodeString(chunkDataStr)
		manifestRef         = swarm.MustParseHexAddress("8504f2a107ca940beafc4ce2f6c9a9f0968c62a5b5893ff0e4e1e2983048d276")
		manifestCh          = swarm.NewChunk(manifestRef, chData)
		look                = newMockLookup(-1, 0, feedUpdate, nil, &id{}, nil)
		factory             = newMockFactory(look)
		bzzDownloadResource = func(addr, path string) string { return "/bzz/" + addr + "/" + path }
		ctx                 = context.Background()
	)
	client, _, _ = newTestServer(t, testServerOptions{
		Storer: storer,
		Tags:   tags.NewTags(mockStatestore, logger),
		Logger: logging.New(os.Stdout, 6),
		Feeds:  factory,
	})
	_, err := storer.Put(ctx, storage.ModePutUpload, feedUpdate)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.NewDefaultManifest(
		loadsave.New(storer, storage.ModePutUpload, false),
		false,
	)
	err = m.Add(ctx, "/", manifest.NewEntry(swarm.ZeroAddress, map[string]string{
		api.FeedMetadataEntryOwner: "abc",
		api.FeedMetadataEntryTopic: "xyz",
		api.FeedMetadataEntryType:  "epoch",
	}))
	if err != nil {
		t.Fatal(err)
	}
	manifRef, err := m.Store(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("manifest addr", manifRef.String())
	nm, err := manifest.NewDefaultManifestReference(
		manifRef,
		loadsave.New(storer, storage.ModePutUpload, false),
	)
	if err != nil {
		t.Fatal(err)
	}
	e, err := nm.Lookup(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(e.Metadata())
	_, err = storer.Put(ctx, storage.ModePutUpload, swarm.NewChunk(feedChunkAddr, feedChunkData))
	if err != nil {
		t.Fatal(err)
	}
	_, err = storer.Put(ctx, storage.ModePutUpload, manifestCh)
	if err != nil {
		t.Fatal(err)
	}

	jsonhttptest.Request(t, client, http.MethodGet, bzzDownloadResource(manifRef.String(), ""), http.StatusOK,
		jsonhttptest.WithExpectedResponse(updateData),
	)
}
