// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/websocket"
	"gitlab.com/nolash/go-mockbytes"
)

type fileUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

func tagsWithIdResource(id uint32) string { return fmt.Sprintf("/tags/%d", id) }

func TestTags(t *testing.T) {

	var (
		bzzResource           = "/bzz"
		bytesResource         = "/bytes"
		chunksResource        = "/chunks"
		tagsResource          = "/tags"
		chunk                 = testingc.GenerateTestRandomChunk()
		mockStatestore        = statestore.NewStateStore()
		logger                = logging.New(ioutil.Discard, 0)
		tag                   = tags.NewTags(mockStatestore, logger)
		client, _, listenAddr = newTestServer(t, testServerOptions{
			Storer: mock.NewStorer(),
			Tags:   tag,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	// list tags without anything pinned
	t.Run("list tags zero", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListTagsResponse{
				Tags: []api.TagResponse{},
			}),
		)
	})

	t.Run("create tag", func(t *testing.T) {
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)
	})

	t.Run("create tag with invalid id", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksResource, http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "cannot get tag",
				Code:    http.StatusBadRequest,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, "invalid_id.jpg"), // the value should be uint32
		)
	})

	t.Run("get invalid tags", func(t *testing.T) {
		// invalid tag
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"/foobar", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid id",
				Code:    http.StatusBadRequest,
			}),
		)

		// non-existent tag
		jsonhttptest.Request(t, client, http.MethodDelete, tagsWithIdResource(uint32(333)), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("create tag upload chunk", func(t *testing.T) {
		// create a tag using the API
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		_ = jsonhttptest.Request(t, client, http.MethodPost, chunksResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
		)

		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, chunksResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithExpectedJSONResponse(api.ChunkAddressResponse{Reference: chunk.Address()}),
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, strconv.FormatUint(uint64(tr.Uid), 10)),
		)

		isTagFoundInResponse(t, rcvdHeaders, &tr)
		tagValueTest(t, tr.Uid, 1, 1, 1, 0, 0, 0, swarm.ZeroAddress, client)
	})

	t.Run("create tag upload chunk stream", func(t *testing.T) {
		// create a tag using the API
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		wsHeaders := http.Header{}
		wsHeaders.Set("Content-Type", "application/octet-stream")
		wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
		wsHeaders.Set(api.SwarmTagHeader, strconv.FormatUint(uint64(tr.Uid), 10))

		u := url.URL{Scheme: "ws", Host: listenAddr, Path: "/chunks/stream"}
		wsConn, _, err := websocket.DefaultDialer.Dial(u.String(), wsHeaders)
		if err != nil {
			t.Fatalf("dial: %v. url %v", err, u.String())
		}

		for i := 0; i < 5; i++ {
			ch := testingc.GenerateTestRandomChunk()

			err := wsConn.SetWriteDeadline(time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			err = wsConn.WriteMessage(websocket.BinaryMessage, ch.Data())
			if err != nil {
				t.Fatal(err)
			}

			err = wsConn.SetReadDeadline(time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			mt, msg, err := wsConn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}

			if mt != websocket.BinaryMessage || !bytes.Equal(msg, api.SuccessWsMsg) {
				t.Fatal("invalid response", mt, string(msg))
			}
		}

		tagValueTest(t, tr.Uid, 5, 5, 0, 0, 0, 0, swarm.ZeroAddress, client)
	})

	t.Run("list tags", func(t *testing.T) {
		// list all current tags
		var resp api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		// create 2 new tags
		tRes1 := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes1),
		)

		tRes2 := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes2),
		)

		expectedTags := []api.TagResponse{
			tRes1,
			tRes2,
		}
		expectedTags = append(expectedTags, resp.Tags...)

		sort.Slice(expectedTags, func(i, j int) bool { return expectedTags[i].Uid < expectedTags[j].Uid })

		// check if listing returns expected tags
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListTagsResponse{
				Tags: expectedTags,
			}),
		)
	})

	t.Run("delete tag error", func(t *testing.T) {
		// try to delete invalid tag
		jsonhttptest.Request(t, client, http.MethodDelete, tagsResource+"/foobar", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid id",
				Code:    http.StatusBadRequest,
			}),
		)

		// try to delete non-existent tag
		jsonhttptest.Request(t, client, http.MethodDelete, tagsWithIdResource(uint32(333)), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("delete tag", func(t *testing.T) {
		// create a tag through API
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)

		// delete tag through API
		jsonhttptest.Request(t, client, http.MethodDelete, tagsWithIdResource(tRes.Uid), http.StatusNoContent,
			jsonhttptest.WithNoResponseBody(),
		)

		// try to get tag
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(tRes.Uid), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("done split error", func(t *testing.T) {
		// invalid tag
		jsonhttptest.Request(t, client, http.MethodPatch, tagsResource+"/foobar", http.StatusBadRequest,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid id",
				Code:    http.StatusBadRequest,
			}),
		)

		// non-existent tag
		jsonhttptest.Request(t, client, http.MethodPatch, tagsWithIdResource(uint32(333)), http.StatusNotFound,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("done split", func(t *testing.T) {
		// create a tag through API
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)
		tagId := tRes.Uid

		// generate address to be supplied to the done split
		addr := test.RandomAddress()

		// upload content with tag
		jsonhttptest.Request(t, client, http.MethodPost, chunksResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(chunk.Data())),
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, fmt.Sprint(tagId)),
		)

		// call done split
		jsonhttptest.Request(t, client, http.MethodPatch, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{
				Address: addr,
			}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "ok",
				Code:    http.StatusOK,
			}),
		)
		tagValueTest(t, tagId, 1, 1, 1, 0, 0, 1, addr, client)

		// try different address value
		addr = test.RandomAddress()

		// call done split
		jsonhttptest.Request(t, client, http.MethodPatch, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{
				Address: addr,
			}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "ok",
				Code:    http.StatusOK,
			}),
		)
		tagValueTest(t, tagId, 1, 1, 1, 0, 0, 1, addr, client)
	})

	t.Run("file tags", func(t *testing.T) {
		// upload a file without supplying tag
		expectedHash := swarm.MustParseHexAddress("40e739ebdfd18292925bba4138cd097db9aa18c1b57e74042f48469b48da33a8")
		expectedResponse := api.BzzUploadResponse{Reference: expectedHash}

		respHeaders := jsonhttptest.Request(t, client, http.MethodPost,
			bzzResource+"?name=somefile", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader([]byte("some data"))),
			jsonhttptest.WithExpectedJSONResponse(expectedResponse),
			jsonhttptest.WithRequestHeader("Content-Type", "application/octet-stream"),
		)

		tagId, err := strconv.Atoi(respHeaders.Get(api.SwarmTagHeader))
		if err != nil {
			t.Fatal(err)
		}
		tagValueTest(t, uint32(tagId), 4, 4, 0, 0, 0, 4, expectedHash, client)
	})

	t.Run("dir tags", func(t *testing.T) {
		// upload a dir without supplying tag
		tarReader := tarFiles(t, []f{{
			data: []byte("some dir data"),
			name: "binary-file",
		}})
		expectedHash := swarm.MustParseHexAddress("42bc27c9137c93705ffbc2945fa1aab0e8e1826f1500b7f06f6e3f86f617213b")
		expectedResponse := api.BzzUploadResponse{Reference: expectedHash}

		respHeaders := jsonhttptest.Request(t, client, http.MethodPost, bzzResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tarReader),
			jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "True"),
			jsonhttptest.WithExpectedJSONResponse(expectedResponse),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
		)

		tagId, err := strconv.Atoi(respHeaders.Get(api.SwarmTagHeader))
		if err != nil {
			t.Fatal(err)
		}
		tagValueTest(t, uint32(tagId), 3, 3, 0, 0, 0, 3, expectedHash, client)
	})

	t.Run("bytes tags", func(t *testing.T) {
		// create a tag using the API
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		sentHeaders := make(http.Header)
		sentHeaders.Set(api.SwarmTagHeader, strconv.FormatUint(uint64(tr.Uid), 10))

		g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
		dataChunk, err := g.SequentialBytes(swarm.ChunkSize)
		if err != nil {
			t.Fatal(err)
		}

		rootAddress := swarm.MustParseHexAddress("5e2a21902f51438be1adbd0e29e1bd34c53a21d3120aefa3c7275129f2f88de9")

		content := make([]byte, swarm.ChunkSize*2)
		copy(content[swarm.ChunkSize:], dataChunk)
		copy(content[:swarm.ChunkSize], dataChunk)

		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, bytesResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			jsonhttptest.WithExpectedJSONResponse(fileUploadResponse{
				Reference: rootAddress,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, strconv.FormatUint(uint64(tr.Uid), 10)),
		)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		tagToVerify, err := tag.Get(id)
		if err != nil {
			t.Fatal(err)
		}

		if tagToVerify.Uid != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", tagToVerify.Uid, tr.Uid)
		}
		tagValueTest(t, id, 3, 3, 1, 0, 0, 3, swarm.ZeroAddress, client)
	})
}

// isTagFoundInResponse verifies that the tag id is found in the supplied HTTP headers
// if an API tag response is supplied, it also verifies that it contains an id which matches the headers
func isTagFoundInResponse(t *testing.T, headers http.Header, tr *api.TagResponse) uint32 {
	t.Helper()

	idStr := headers.Get(api.SwarmTagHeader)
	if idStr == "" {
		t.Fatalf("could not find tag id header in chunk upload response")
	}
	nId, err := strconv.Atoi(idStr)
	id := uint32(nId)
	if err != nil {
		t.Fatal(err)
	}
	if tr != nil {
		if id != tr.Uid {
			t.Fatalf("expected created tag id to be %d, but got %d when uploading chunk", tr.Uid, id)
		}
	}
	return id
}

func tagValueTest(t *testing.T, id uint32, split, stored, seen, sent, synced, total int64, address swarm.Address, client *http.Client) {
	t.Helper()
	tag := api.TagResponse{}
	jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(id), http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&tag),
	)

	if tag.Processed != stored {
		t.Errorf("tag processed count mismatch. got %d want %d", tag.Processed, stored)
	}
	if tag.Synced != seen+synced {
		t.Errorf("tag synced count mismatch. got %d want %d (seen: %d, synced: %d)", tag.Synced, seen+synced, seen, synced)
	}
	if tag.Total != total {
		t.Errorf("tag total count mismatch. got %d want %d", tag.Total, total)
	}
}
