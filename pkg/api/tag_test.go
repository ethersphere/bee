// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	mp "github.com/ethersphere/bee/pkg/pusher/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/tags"
	"gitlab.com/nolash/go-mockbytes"
)

type fileUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

func TestTags(t *testing.T) {
	var (
		filesResource      = "/files"
		dirResource        = "/dirs"
		bytesResource      = "/bytes"
		chunksResource     = func(addr swarm.Address) string { return "/chunks/" + addr.String() }
		tagsResource       = "/tags"
		tagsWithIdResource = func(id uint32) string { return fmt.Sprintf("/tags/%d", id) }
		someHash           = swarm.MustParseHexAddress("aabbcc")
		someContent        = []byte("bbaatt")
		someTagName        = "file.jpg"
		tag                = tags.NewTags()
		mockPusher         = mp.NewMockPusher(tag)
		client             = newTestServer(t, testServerOptions{
			Storer: mock.NewStorer(),
			Tags:   tag,
		})
	)

	t.Run("create-unnamed-tag", func(t *testing.T) {
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		if !strings.Contains(tr.Name, "unnamed_tag_") {
			t.Fatalf("expected tag name to contain %s but is %s instead", "unnamed_tag_", tr.Name)
		}
	})

	t.Run("create-tag-with-name", func(t *testing.T) {
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{
				Name: someTagName,
			}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		if tr.Name != someTagName {
			t.Fatalf("expected tag name to be %s but is %s instead", someTagName, tr.Name)
		}
	})

	t.Run("create-tag-from-chunk-upload-with-invalid-id", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusInternalServerError,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "cannot get or create tag",
				Code:    http.StatusInternalServerError,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagUidHeader, "invalid_id.jpg"), // the value should be uint32
		)
	})

	t.Run("get-invalid-tags", func(t *testing.T) {
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

	t.Run("get-tag-id-from-chunk-upload-without-tag", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		isTagFoundInResponse(t, rcvdHeaders, nil)
	})

	t.Run("create-tag-and-use-it-to-upload-chunk", func(t *testing.T) {
		// create a tag using the API
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{
				Name: someTagName,
			}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		if tr.Name != someTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", someTagName, tr.Name)
		}

		// now upload a chunk and see if we receive a tag with the same id
		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagUidHeader, strconv.FormatUint(uint64(tr.Uid), 10)),
		)

		isTagFoundInResponse(t, rcvdHeaders, &tr)
	})

	t.Run("create-tag-and-use-it-to-upload-multiple-chunks", func(t *testing.T) {
		// create a tag using the API
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{
				Name: someTagName,
			}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		if tr.Name != someTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", someTagName, tr.Name)
		}

		// now upload a chunk and see if we receive a tag with the same id
		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagUidHeader, fmt.Sprint(tr.Uid)),
		)

		isTagFoundInResponse(t, rcvdHeaders, &tr)

		// add a second valid content validator
		secondValidHash := swarm.MustParseHexAddress("deadbeaf")
		secondValidContent := []byte("123456")

		rcvdHeaders = jsonhttptest.Request(t, client, http.MethodPost, chunksResource(secondValidHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(secondValidContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagUidHeader, fmt.Sprint(tr.Uid)),
		)

		isTagFoundInResponse(t, rcvdHeaders, &tr)
	})

	t.Run("get-tag-from-chunk-upload-and-use-it-again", func(t *testing.T) {
		// upload a new chunk and get the generated tag id
		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)

		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		// see if the tag id is present and has valid values
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(id), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		// now upload another chunk using the same tag id
		jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagUidHeader, fmt.Sprint(tr.Uid)),
		)

		// see if the tag id is present and has valid values
		tr = api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(id), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)

		if id != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", id, tr.Uid)
		}
		if tr.Stored != 2 {
			t.Fatalf("expected stored counter to be %d but is %d", 2, tr.Stored)
		}
	})

	t.Run("get-tag-using-id", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		// request the tag and see if the ID is the same
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(id), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)
		if id != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", id, tr.Uid)
		}
	})

	t.Run("tag-counters", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: http.StatusText(http.StatusOK),
				Code:    http.StatusOK,
			}),
		)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		tagToVerify, err := tag.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		err = mockPusher.SendChunk(id)
		if err != nil {
			t.Fatal(err)
		}
		err = mockPusher.RcvdReceipt(id)
		if err != nil {
			t.Fatal(err)
		}

		finalTag := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(id), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&finalTag),
		)

		if tagToVerify.Total != finalTag.Total {
			t.Errorf("tag total count mismatch. got %d want %d", tagToVerify.Total, finalTag.Total)
		}
		if tagToVerify.Seen != finalTag.Seen {
			t.Errorf("tag seen count mismatch. got %d want %d", tagToVerify.Seen, finalTag.Seen)
		}
		if tagToVerify.Stored != finalTag.Stored {
			t.Errorf("tag stored count mismatch. got %d want %d", tagToVerify.Stored, finalTag.Stored)
		}
		if tagToVerify.Sent != finalTag.Sent {
			t.Errorf("tag sent count mismatch. got %d want %d", tagToVerify.Sent, finalTag.Sent)
		}
		if tagToVerify.Synced != finalTag.Synced {
			t.Errorf("tag synced count mismatch. got %d want %d", tagToVerify.Synced, finalTag.Synced)
		}
	})

	t.Run("delete-tag-error", func(t *testing.T) {
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

	t.Run("delete-tag", func(t *testing.T) {
		// create a tag through API
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{
				Name: someTagName,
			}),
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

	t.Run("done-split-error", func(t *testing.T) {
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

	t.Run("done-split", func(t *testing.T) {
		// create a tag through API
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{
				Name: someTagName,
			}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)
		tagId := tRes.Uid

		// generate address to be supplied to the done split
		addr := test.RandomAddress()

		// upload content with tag
		jsonhttptest.Request(t, client, http.MethodPost, chunksResource(someHash), http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(someContent)),
			jsonhttptest.WithRequestHeader(api.SwarmTagUidHeader, fmt.Sprint(tagId)),
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

		// check tag data
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)
		if !tRes.Address.Equal(addr) {
			t.Fatalf("expected tag address to be %s but is %s", addr.String(), tRes.Address.String())
		}
		total := tRes.Total
		if !(total > 0) {
			t.Errorf("tag total should be greater than 0 but it is not")
		}

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

		// check tag data
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)
		if !tRes.Address.Equal(addr) {
			t.Fatalf("expected tag address to be %s but is %s", addr.String(), tRes.Address.String())
		}
		if tRes.Total != total {
			t.Errorf("tag total should not have changed")
		}
	})

	t.Run("file-tags", func(t *testing.T) {
		// upload a file without supplying tag
		expectedHash := swarm.MustParseHexAddress("8e27bb803ff049e8c2f4650357026723220170c15ebf9b635a7026539879a1a8")
		expectedResponse := api.FileUploadResponse{Reference: expectedHash}

		respHeaders := jsonhttptest.Request(t, client, http.MethodPost, filesResource, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader([]byte("some data"))),
			jsonhttptest.WithExpectedJSONResponse(expectedResponse),
			jsonhttptest.WithRequestHeader("Content-Type", "application/octet-stream"),
		)

		tagId, err := strconv.Atoi(respHeaders.Get(api.SwarmTagUidHeader))
		if err != nil {
			t.Fatal(err)
		}

		// check tag data
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(uint32(tagId)), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)

		if !(tRes.Total > 0) {
			t.Errorf("tag total should be greater than 0 but it is not")
		}
		if !(tRes.Stored > 0) {
			t.Errorf("tag stored should be greater than 0 but it is not")
		}
		if !(tRes.Split > 0) {
			t.Errorf("tag split should be greater than 0 but it is not")
		}
	})

	t.Run("dir-tags", func(t *testing.T) {
		// upload a dir without supplying tag
		tarReader := tarFiles(t, []f{{
			data: []byte("some data"),
			name: "binary-file",
		}})

		expectedHash := swarm.MustParseHexAddress("ebcfbfac0e9a4fa4483491875f9486107a799e54cd832d0aacc59b1125b4b71f")
		expectedResponse := api.FileUploadResponse{Reference: expectedHash}

		respHeaders := jsonhttptest.Request(t, client, http.MethodPost, dirResource, http.StatusOK,
			jsonhttptest.WithRequestBody(tarReader),
			jsonhttptest.WithExpectedJSONResponse(expectedResponse),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
		)

		tagId, err := strconv.Atoi(respHeaders.Get(api.SwarmTagUidHeader))
		if err != nil {
			t.Fatal(err)
		}

		// check tag data
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(uint32(tagId)), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)

		if !(tRes.Total > 0) {
			t.Errorf("tag total should be greater than 0 but it is not")
		}
		if !(tRes.Stored > 0) {
			t.Errorf("tag stored should be greater than 0 but it is not")
		}
		if !(tRes.Split > 0) {
			t.Errorf("tag split should be greater than 0 but it is not")
		}
	})

	t.Run("bytes-tags", func(t *testing.T) {
		// create a tag using the API
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{
				Name: someTagName,
			}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)
		if tr.Name != someTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", someTagName, tr.Name)
		}

		sentHeaders := make(http.Header)
		sentHeaders.Set(api.SwarmTagUidHeader, strconv.FormatUint(uint64(tr.Uid), 10))

		g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
		dataChunk, err := g.SequentialBytes(swarm.ChunkSize)
		if err != nil {
			t.Fatal(err)
		}

		rootAddress := swarm.MustParseHexAddress("5e2a21902f51438be1adbd0e29e1bd34c53a21d3120aefa3c7275129f2f88de9")

		content := make([]byte, swarm.ChunkSize*2)
		copy(content[swarm.ChunkSize:], dataChunk)
		copy(content[:swarm.ChunkSize], dataChunk)

		rcvdHeaders := jsonhttptest.Request(t, client, http.MethodPost, bytesResource, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			jsonhttptest.WithExpectedJSONResponse(fileUploadResponse{
				Reference: rootAddress,
			}),
			jsonhttptest.WithRequestHeader(api.SwarmTagUidHeader, strconv.FormatUint(uint64(tr.Uid), 10)),
		)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		tagToVerify, err := tag.Get(id)
		if err != nil {
			t.Fatal(err)
		}

		if tagToVerify.Uid != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", tagToVerify.Uid, tr.Uid)
		}

		finalTag := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(id), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&finalTag),
		)

		if finalTag.Total != 0 {
			t.Errorf("tag total count mismatch. got %d want %d", finalTag.Total, 0)
		}
		if finalTag.Seen != 1 {
			t.Errorf("tag seen count mismatch. got %d want %d", finalTag.Seen, 1)
		}
		if finalTag.Stored != 3 {
			t.Errorf("tag stored count mismatch. got %d want %d", finalTag.Stored, 3)
		}

		if !finalTag.Address.Equal(swarm.ZeroAddress) {
			t.Errorf("address mismatch: expected %s got %s", rootAddress.String(), finalTag.Address.String())
		}
	})
}

// isTagFoundInResponse verifies that the tag id is found in the supplied HTTP headers
// if an API tag response is supplied, it also verifies that it contains an id which matches the headers
func isTagFoundInResponse(t *testing.T, headers http.Header, tr *api.TagResponse) uint32 {
	idStr := headers.Get(api.SwarmTagUidHeader)
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
