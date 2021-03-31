// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	// "encoding/hex"
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
	emptyAddr := make([]byte, 32)
	err = m.Add(ctx, "/", manifest.NewEntry(swarm.NewAddress(emptyAddr), map[string]string{
		api.FeedMetadataEntryOwner: "8d3766440f0d7b949a5e32995d09619a7f86e632",
		api.FeedMetadataEntryTopic: "abcc",
		api.FeedMetadataEntryType:  "epoch",
	}))
	if err != nil {
		t.Fatal(err)
	}
	manifRef, err := m.Store(ctx)
	if err != nil {
		t.Fatal(err)
	}

	jsonhttptest.Request(t, client, http.MethodGet, bzzDownloadResource(manifRef.String(), ""), http.StatusOK,
		jsonhttptest.WithExpectedResponse(updateData),
	)
}
