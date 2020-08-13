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
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	mp "github.com/ethersphere/bee/pkg/pusher/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"gitlab.com/nolash/go-mockbytes"
)

type fileUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

func TestTags(t *testing.T) {
	var (
		bytesResource        = "/bytes"
		chunksResource       = func(addr swarm.Address) string { return "/chunks/" + addr.String() }
		tagsResource         = "/tags"
		tagsWithIdResource   = func(id uint64) string { return fmt.Sprintf("/tags/%d", id) }
		validHash            = swarm.MustParseHexAddress("aabbcc")
		validContent         = []byte("bbaatt")
		validTagName         = "file.jpg"
		mockValidator        = validator.NewMockValidator(validHash, validContent)
		tag                  = tags.NewTags()
		mockValidatingStorer = mock.NewValidatingStorer(mockValidator, tag)
		mockPusher           = mp.NewMockPusher(tag)
		client               = newTestServer(t, testServerOptions{
			Storer: mockValidatingStorer,
			Tags:   tag,
		})
	)

	t.Run("create-tag-with-invalid-id", func(t *testing.T) {
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.TagHeaderUid, "invalid_id.jpg") // the value should be uint32
		_ = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "cannot get or create tag",
			Code:    http.StatusInternalServerError,
		}, sentHeaders)
	})

	t.Run("get-tag-id-from-chunk-upload-without-tag", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)

		isTagFoundInResponse(t, rcvdHeaders, nil)
	})

	t.Run("create-tag-and-use-it-to-upload-chunk", func(t *testing.T) {
		// create a tag using the API
		b, err := json.Marshal(api.TagResponse{
			Name: validTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)

		if tr.Name != validTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", validTagName, tr.Name)
		}

		// now upload a chunk and see if we receive a tag with the same id
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(tr.Uid), 10))
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		isTagFoundInResponse(t, rcvdHeaders, &tr)
	})

	t.Run("create-tag-and-use-it-to-upload-multiple-chunks", func(t *testing.T) {
		// create a tag using the API
		b, err := json.Marshal(api.TagResponse{
			Name: validTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)

		if tr.Name != validTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", validTagName, tr.Name)
		}

		// now upload a chunk and see if we receive a tag with the same id
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(tr.Uid), 10))
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		isTagFoundInResponse(t, rcvdHeaders, &tr)

		// add a second valid content validator
		secondValidHash := swarm.MustParseHexAddress("deadbeaf")
		secondValidContent := []byte("123456")
		mockValidator.AddPair(secondValidHash, secondValidContent)

		sentHeaders = make(http.Header)
		sentHeaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(tr.Uid), 10))
		rcvdHeaders = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(secondValidHash), bytes.NewReader(secondValidContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		isTagFoundInResponse(t, rcvdHeaders, &tr)
	})

	t.Run("get-tag-from-chunk-upload-and-use-it-again", func(t *testing.T) {
		// upload a new chunk and get the generated tag id
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)

		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		// see if the tag id is present and has valid values
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &tr)

		// now upload another chunk using the same tag id
		sentHeaders := make(http.Header)
		sentHeaders.Set(api.TagHeaderUid, strconv.FormatUint(id, 10))
		_ = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHeaders)

		// see if the tag id is present and has valid values
		tr = api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &tr)

		if id != uint64(tr.Uid) {
			t.Fatalf("expected tag id to be %d but is %d", id, tr.Uid)
		}
		if tr.Stored != 2 {
			t.Fatalf("expected stored counter to be %d but is %d", 2, tr.Stored)
		}
	})

	t.Run("get-tag-using-id", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		// request the tag and see if the ID is the same
		tr := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(id), nil, http.StatusOK, &tr)
		if id != uint64(tr.Uid) {
			t.Fatalf("expected tag id to be %d but is %d", id, tr.Uid)
		}
	})

	t.Run("tag-counters", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, chunksResource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)
		id := isTagFoundInResponse(t, rcvdHeaders, nil)

		tagToVerify, err := tag.Get(uint32(id))
		if err != nil {
			t.Fatal(err)
		}
		err = mockPusher.SendChunk(uint32(id))
		if err != nil {
			t.Fatal(err)
		}
		err = mockPusher.RcvdReceipt(uint32(id))
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

	t.Run("bytes-tag-counters", func(t *testing.T) {
		// create a tag using the API
		tr := api.TagResponse{}
		b, err := json.Marshal(api.TagResponse{
			Name: validTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tr)
		if tr.Name != validTagName {
			t.Fatalf("sent tag name %s does not match received tag name %s", validTagName, tr.Name)
		}

		sentHeaders := make(http.Header)
		sentHeaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(tr.Uid), 10))

		g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
		dataChunk, err := g.SequentialBytes(swarm.ChunkSize)
		if err != nil {
			t.Fatal(err)
		}

		chunkAddress := swarm.MustParseHexAddress("c10090961e7682a10890c334d759a28426647141213abda93b096b892824d2ef")
		rootBytes := swarm.MustParseHexAddress("c10090961e7682a10890c334d759a28426647141213abda93b096b892824d2ef").Bytes()
		rootChunk := make([]byte, 64)
		copy(rootChunk[:32], rootBytes)
		copy(rootChunk[32:], rootBytes)
		rootAddress := swarm.MustParseHexAddress("5e2a21902f51438be1adbd0e29e1bd34c53a21d3120aefa3c7275129f2f88de9")

		mockValidator.AddPair(chunkAddress, dataChunk)
		mockValidator.AddPair(rootAddress, rootChunk)

		content := make([]byte, swarm.ChunkSize*2)
		copy(content[swarm.ChunkSize:], dataChunk)
		copy(content[:swarm.ChunkSize], dataChunk)

		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, bytesResource, bytes.NewReader(content), http.StatusOK, fileUploadResponse{
			Reference: rootAddress,
		}, sentHeaders)
		uuid := isTagFoundInResponse(t, rcvdHeaders, nil)

		tagToVerify, err := tag.Get(uint32(uuid))
		if err != nil {
			t.Fatal(err)
		}

		if tagToVerify.Uid != tr.Uid {
			t.Fatalf("expected tag id to be %d but is %d", tagToVerify.Uid, tr.Uid)
		}

		finalTag := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagsWithIdResource(uuid), nil, http.StatusOK, &finalTag)

		if finalTag.Total != 0 {
			t.Errorf("tag total count mismatch. got %d want %d", finalTag.Total, 0)
		}
		if finalTag.Seen != 3 {
			t.Errorf("tag seen count mismatch. got %d want %d", finalTag.Seen, 3)
		}
		if finalTag.Stored != 3 {
			t.Errorf("tag stored count mismatch. got %d want %d", finalTag.Stored, 3)
		}

		if !finalTag.Address.Equal(swarm.ZeroAddress) {
			t.Errorf("address mismatch: expected %s got %s", rootAddress.String(), finalTag.Address.String())
		}

	})

	t.Run("delete-tag", func(t *testing.T) {
		// create a tag through API
		b, err := json.Marshal(api.TagResponse{
			Name: validTagName,
		})
		if err != nil {
			t.Fatal(err)
		}
		tRes := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagsResource, bytes.NewReader(b), http.StatusCreated, &tRes)

		// delete tag through API
		jsonhttptest.ResponseDirect(t, client, http.MethodDelete, tagsWithIdResource(uint64(tRes.Uid)), nil, http.StatusNoContent, jsonhttp.StatusResponse{
			Message: "ok",
			Code:    http.StatusNoContent,
		})
	})
}

// isTagFoundInResponse verifies that the tag id is found in the supplied HTTP headers
// if an API tag response is supplied, it also verifies that it contains an id which matches the headers
func isTagFoundInResponse(t *testing.T, headers http.Header, tr *api.TagResponse) uint64 {
	idStr := headers.Get(api.TagHeaderUid)
	if idStr == "" {
		t.Fatalf("could not find tag id header in chunk upload response")
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	if tr != nil {
		if id != uint64(tr.Uid) {
			t.Fatalf("expected created tag id to be %d, but got %d when uploading chunk", tr.Uid, id)
		}
	}
	return id
}
