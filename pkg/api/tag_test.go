// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/json"
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
		tReq := &api.TagRequest{}
		b, err := json.Marshal(tReq)
		if err != nil {
			t.Fatal(err)
		}

		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)

		if !strings.Contains(tr.Name, "unnamed_tag_") {
			t.Fatalf("expected tag name to contain %s but is %s instead", "unnamed_tag_", tr.Name)
		}
	})

	t.Run("create-tag-with-name", func(t *testing.T) {
		tReq := &api.TagRequest{
			Name: someTagName,
		}
		b, err := json.Marshal(tReq)
		if err != nil {
			t.Fatal(err)
		}

		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)

		if tr.Name != someTagName {
			t.Fatalf("expected tag name to be %s but is %s instead", someTagName, tr.Name)
		}
	})

	t.Run("create-tag-from-chunk-upload-with-invalid-id", func(t *testing.T) {
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.SwarmTagUidHeader, "invalid_id.jpg") // the value should be uint32
		_ = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "cannot get or create tag",
			Code:    http.StatusInternalServerError,
		}, sentHeaders)
	})

	t.Run("get-invalid-tags", func(t *testing.T) {
		// invalid tag
		jsonhttptest.ResponseDirect(t, client, http.MethodGet, tagsResource+"/foobar", nil, http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "invalid id",
			Code:    http.StatusBadRequest,
		})

		// non-existent tag
		jsonhttptest.ResponseDirect(t, client, http.MethodDelete, tagsWithIdResource(uint32(333)), nil, http.StatusNotFound, jsonhttp.StatusResponse{
			Message: "tag not present",
			Code:    http.StatusNotFound,
		})
	})

	t.Run("get-tag-id-from-chunk-upload-without-tag", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)

		isTagFoundInResponse(t, rcvdHeaders, nil)
	})

	t.Run("create-tag-and-use-it-to-upload-chunk", func(t *testing.T) {
		// create a tag using the API
		b, err := json.Marshal(api.TagResponse{
			Name: someTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)

		if tr.Name != someTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", someTagName, tr.Name)
		}

		// now upload a chunk and see if we receive a tag with the same id
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.SwarmTagUidHeader, strconv.FormatUint(uint64(tr.Uid), 10))
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		isTagFoundInResponse(t, rcvdHeaders, &tr)
	})

	t.Run("create-tag-and-use-it-to-upload-multiple-chunks", func(t *testing.T) {
		// create a tag using the API
		b, err := json.Marshal(api.TagResponse{
			Name: someTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)

		if tr.Name != someTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", someTagName, tr.Name)
		}

		// now upload a chunk and see if we receive a tag with the same id
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.SwarmTagUidHeader, fmt.Sprint(tr.Uid))
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		isTagFoundInResponse(t, rcvdHeaders, &tr)

		// add a second valid content validator
		secondValidHash := swarm.MustParseHexAddress("deadbeaf")
		secondValidContent := []byte("123456")

		sentHeaders = make(http.Header)
		sentHeaders.Set(api.SwarmTagUidHeader, fmt.Sprint(tr.Uid))
		rcvdHeaders = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(secondValidHash), bytes.NewReader(secondValidContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		isTagFoundInResponse(t, rcvdHeaders, &tr)
	})

	t.Run("get-tag-from-chunk-upload-and-use-it-again", func(t *testing.T) {
		// upload a new chunk and get the generated tag id
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)

		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		// see if the tag id is present and has valid values
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &tr)

		// now upload another chunk using the same tag id
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.SwarmTagUidHeader, fmt.Sprint(tr.Uid))
		_ = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		// see if the tag id is present and has valid values
		tr = api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &tr)

		if id != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", id, tr.Uid)
		}
		if tr.Stored != 2 {
			t.Fatalf("expected stored counter to be %d but is %d", 2, tr.Stored)
		}
	})

	t.Run("get-tag-using-id", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		// request the tag and see if the ID is the same
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &tr)
		if id != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", id, tr.Uid)
		}
	})

	t.Run("tag-counters", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)
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
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &finalTag)

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
		jsonhttptest.ResponseDirect(t, client, http.MethodDelete, tagsResource+"/foobar", nil, http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "invalid id",
			Code:    http.StatusBadRequest,
		})

		// try to delete non-existent tag
		jsonhttptest.ResponseDirect(t, client, http.MethodDelete, tagsWithIdResource(uint32(333)), nil, http.StatusNotFound, jsonhttp.StatusResponse{
			Message: "tag not present",
			Code:    http.StatusNotFound,
		})
	})

	t.Run("delete-tag", func(t *testing.T) {
		// create a tag through API
		b, err := json.Marshal(api.TagResponse{
			Name: someTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		tRes := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tRes)

		// delete tag through API
		jsonhttptest.ResponseDirect(t, client, http.MethodDelete, tagsWithIdResource(tRes.Uid), nil, http.StatusNoContent, nil)

		// try to get tag
		jsonhttptest.ResponseDirect(t, client, http.MethodGet, tagsWithIdResource(tRes.Uid), nil, http.StatusNotFound, jsonhttp.StatusResponse{
			Message: "tag not present",
			Code:    http.StatusNotFound,
		})
	})

	t.Run("done-split-error", func(t *testing.T) {
		b, err := json.Marshal(api.TagResponse{})
		if err != nil {
			t.Fatal(err)
		}

		// invalid tag
		jsonhttptest.ResponseDirect(t, client, http.MethodPatch, tagsResource+"/foobar", bytes.NewReader(b), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "invalid id",
			Code:    http.StatusBadRequest,
		})

		// non-existent tag
		jsonhttptest.ResponseDirect(t, client, http.MethodPatch, tagsWithIdResource(uint32(333)), bytes.NewReader(b), http.StatusNotFound, jsonhttp.StatusResponse{
			Message: "tag not present",
			Code:    http.StatusNotFound,
		})
	})

	t.Run("done-split", func(t *testing.T) {
		// create a tag through API
		tResB, err := json.Marshal(api.TagResponse{
			Name: someTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		tRes := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(tResB), http.StatusCreated, &tRes)
		tagId := tRes.Uid

		// generate address to be supplied to the done split
		addr := test.RandomAddress()
		tReqB, err := json.Marshal(api.TagRequest{
			Address: addr,
		})
		if err != nil {
			t.Fatal(err)
		}

		// upload content with tag
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.SwarmTagUidHeader, fmt.Sprint(tagId))
		jsonhttptest.ResponseDirectSendHeadersAndDontCheckResponse(t, client, http.MethodPost, chunksResource(someHash), bytes.NewReader(someContent), http.StatusOK, sentHeaders)

		// call done split
		jsonhttptest.ResponseDirect(t, client, http.MethodPatch, tagsWithIdResource(tagId), bytes.NewReader(tReqB), http.StatusOK, jsonhttp.StatusResponse{
			Message: "ok",
			Code:    http.StatusOK,
		})

		// check tag data
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(tagId), nil, http.StatusOK, &tRes)
		if !tRes.Address.Equal(addr) {
			t.Fatalf("expected tag address to be %s but is %s", addr.String(), tRes.Address.String())
		}
		total := tRes.Total
		if !(total > 0) {
			t.Errorf("tag total should be greater than 0 but it is not")
		}

		// try different address value
		addr = test.RandomAddress()
		tReqB, err = json.Marshal(api.TagRequest{
			Address: addr,
		})
		if err != nil {
			t.Fatal(err)
		}

		// call done split
		jsonhttptest.ResponseDirect(t, client, http.MethodPatch, tagsWithIdResource(tagId), bytes.NewReader(tReqB), http.StatusOK, jsonhttp.StatusResponse{
			Message: "ok",
			Code:    http.StatusOK,
		})

		// check tag data
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(tagId), nil, http.StatusOK, &tRes)
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

		sentHeaders := make(http.Header)
		sentHeaders.Set("Content-Type", "application/octet-stream")
		respHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, filesResource, bytes.NewReader([]byte("some data")), http.StatusOK, expectedResponse, sentHeaders)

		tagId, err := strconv.Atoi(respHeaders.Get(api.SwarmTagUidHeader))
		if err != nil {
			t.Fatal(err)
		}

		// check tag data
		tRes := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(uint32(tagId)), nil, http.StatusOK, &tRes)

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

		expectedHash := swarm.MustParseHexAddress("9e5acfbfeb7e074d4c79f5f9922e8a25990dad267d0ea7becaaad07b47fb2a87")
		expectedResponse := api.FileUploadResponse{Reference: expectedHash}

		sentHeaders := make(http.Header)
		sentHeaders.Set("Content-Type", api.ContentTypeTar)
		respHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirResource, tarReader, http.StatusOK, expectedResponse, sentHeaders)

		tagId, err := strconv.Atoi(respHeaders.Get(api.SwarmTagUidHeader))
		if err != nil {
			t.Fatal(err)
		}

		// check tag data
		tRes := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(uint32(tagId)), nil, http.StatusOK, &tRes)

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
		b, err := json.Marshal(api.TagResponse{
			Name: someTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)
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

		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, bytesResource, bytes.NewReader(content), http.StatusOK, fileUploadResponse{
			Reference: rootAddress,
		}, sentHeaders)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		tagToVerify, err := tag.Get(id)
		if err != nil {
			t.Fatal(err)
		}

		if tagToVerify.Uid != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", tagToVerify.Uid, tr.Uid)
		}

		finalTag := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &finalTag)

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
