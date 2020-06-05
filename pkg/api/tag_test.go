// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
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
)

func TestTags(t *testing.T) {
	var (
		resource             = func(addr swarm.Address) string { return "/bzz-chunk/" + addr.String() }
		tagResourceAddress   = func(addr swarm.Address) string { return "/bzz-tag/addr/" + addr.String() }
		tagResourceUidCreate = func(name string) string { return "/bzz-tag/name/" + name }
		tagResourceUUid      = func(uuid uint64) string { return "/bzz-tag/uuid/" + strconv.FormatUint(uuid, 10) }
		validHash            = swarm.MustParseHexAddress("aabbcc")
		validContent         = []byte("bbaatt")
		mockValidator        = validator.NewMockValidator(validHash, append(newSpan(uint64(len(validContent))), validContent...))
		tag                  = tags.NewTags()
		mockValidatingStorer = mock.NewValidatingStorer(mockValidator, tag)
		mockPusher           = mp.NewMockPusher(tag)
		client               = newTestServer(t, testServerOptions{
			Storer: mockValidatingStorer,
			Tags:   tag,
		})
	)

	t.Run("send-invalid-tag-id", func(t *testing.T) {
		sentHheaders := make(http.Header)
		sentHheaders.Set(api.TagHeaderUid, "file.jpg") // the value should be uint32
		_ = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "invalid taguid",
			Code:    http.StatusBadRequest,
		}, sentHheaders)
	})

	t.Run("uid-header-in-return-for-empty-tag", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)

		isTagFoundInResponse(t, rcvdHeaders, nil)
	})

	t.Run("get-tag-and-use-it-to-upload-chunk", func(t *testing.T) {
		// Get a tag using API
		ta := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagResourceUidCreate("file.jpg"), nil, http.StatusOK, &ta)

		if ta.Name != "file.jpg" {
			t.Fatalf("tagname is not the same that we sent")
		}

		// Now upload a chunk and see if we receive a tag with the same uid
		sentHheaders := make(http.Header)
		sentHheaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(ta.Uid), 10))
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHheaders)

		isTagFoundInResponse(t, rcvdHeaders, &ta)
	})

	t.Run("get-tag-and-use-it-to-upload-multiple-chunk", func(t *testing.T) {
		// Get a tag using API
		ta := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagResourceUidCreate("file.jpg"), nil, http.StatusOK, &ta)

		if ta.Name != "file.jpg" {
			t.Fatalf("tagname is not the same that we sent")
		}

		// Now upload a chunk and see if we receive a tag with the same uid
		sentHheaders := make(http.Header)
		sentHheaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(ta.Uid), 10))
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHheaders)

		isTagFoundInResponse(t, rcvdHeaders, &ta)

		// Add asecond valid contentto validator
		secondValidHash := swarm.MustParseHexAddress("deadbeaf")
		secondValidContent := []byte("123456")
		mockValidator.AddPair(secondValidHash, append(newSpan(uint64(len(secondValidContent))), secondValidContent...))

		sentHheaders = make(http.Header)
		sentHheaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(ta.Uid), 10))
		rcvdHeaders = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(secondValidHash), bytes.NewReader(secondValidContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHheaders)

		isTagFoundInResponse(t, rcvdHeaders, &ta)
	})

	t.Run("get-tag-indirectly-and-use-it-to-upload-chunk", func(t *testing.T) {
		//Upload anew chunk and we give aUID in response and apps can use that too
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)

		uuid := isTagFoundInResponse(t, rcvdHeaders, nil)

		// see if the tagid is present and has valid values
		ta := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagResourceUUid(uuid), nil, http.StatusOK, &ta)

		// Now upload another chunk using the same tag id
		sentHheaders := make(http.Header)
		sentHheaders.Set(api.TagHeaderUid, strconv.FormatUint(uuid, 10))
		_ = jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHheaders)

		// see if the tagid is present and has valid values
		ta = api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagResourceUUid(uuid), nil, http.StatusOK, &ta)

		if uuid != uint64(ta.Uid) {
			t.Fatalf("Invalid uuid response")
		}
		if ta.Stored != 2 {
			t.Fatalf("same tag not used")
		}
	})

	t.Run("get-tag-using-address", func(t *testing.T) {
		// Get a tag
		ta := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodPost, tagResourceUidCreate("file.jpg"), nil, http.StatusOK, &ta)

		if ta.Name != "file.jpg" {
			t.Fatalf("tagname is not the same that we sent")
		}

		// Now upload a chunk and see if we receive a tag with the same uid
		sentHheaders := make(http.Header)
		sentHheaders.Set(api.TagHeaderUid, strconv.FormatUint(uint64(ta.Uid), 10))
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, sentHheaders)

		uuid := isTagFoundInResponse(t, rcvdHeaders, &ta)

		// Request the tag and see if the UUID is the same
		rtag := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagResourceAddress(validHash), nil, http.StatusOK, &rtag)

		if uuid != uint64(rtag.Uid) {
			t.Fatalf("Invalid uuid response")
		}
	})

	t.Run("get-tag-using-uuid", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)
		uuid := isTagFoundInResponse(t, rcvdHeaders, nil)

		// Request the tag and see if the UUID is the same
		ta := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagResourceUUid(uuid), nil, http.StatusOK, &ta)
		if uuid != uint64(ta.Uid) {
			t.Fatalf("Invalid uuid response")
		}
	})

	t.Run("tag-counters", func(t *testing.T) {
		rcvdHeaders := jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}, nil)
		uuid1 := isTagFoundInResponse(t, rcvdHeaders, nil)

		tagToVerify, err := tag.Get(uint32(uuid1))
		if err != nil {
			t.Fatal(err)
		}
		err = mockPusher.SendChunk(validHash)
		if err != nil {
			t.Fatal(err)
		}
		err = mockPusher.RcvdReceipt(validHash)
		if err != nil {
			t.Fatal(err)
		}

		finalTag := api.TagResponse{}
		jsonhttptest.ResponseUnmarshal(t, client, http.MethodGet, tagResourceUUid(uuid1), nil, http.StatusOK, &finalTag)

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
}

func isTagFoundInResponse(t *testing.T, headers http.Header, tag *api.TagResponse) uint64 {
	uidStr := headers.Get(api.TagHeaderUid)
	if uidStr == "" {
		t.Fatalf("could not find tagid header in chunk upload response")
	}
	uid, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	if tag != nil {
		if uid != uint64(tag.Uid) {
			t.Fatalf("uid created is not received while uploading chunk, expected : %d, got %d", tag.Uid, uid)
		}
	}
	return uid
}
